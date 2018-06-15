/*
Copyright (c) 2018 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package session

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/vmware/govmomi/govc/cli"
	"github.com/vmware/govmomi/govc/flags"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
)

type login struct {
	*flags.ClientFlag
	*flags.OutputFlag

	clone  bool
	long   bool
	ticket string
	cookie string
}

func init() {
	cli.Register("session.login", &login{})
}

func (cmd *login) Register(ctx context.Context, f *flag.FlagSet) {
	cmd.ClientFlag, ctx = flags.NewClientFlag(ctx)
	cmd.ClientFlag.Register(ctx, f)
	cmd.OutputFlag, ctx = flags.NewOutputFlag(ctx)
	cmd.OutputFlag.Register(ctx, f)

	f.BoolVar(&cmd.clone, "clone", false, "Acquire clone ticket")
	f.BoolVar(&cmd.long, "l", false, "Output session cookie")
	f.StringVar(&cmd.ticket, "ticket", "", "Clone ticket")
	f.StringVar(&cmd.cookie, "cookie", "", "Set HTTP cookie for an existing session")
}

func (cmd *login) Process(ctx context.Context) error {
	if err := cmd.OutputFlag.Process(ctx); err != nil {
		return err
	}
	return cmd.ClientFlag.Process(ctx)
}

func (cmd *login) Description() string {
	return `Session login.

The session.login command is optional, all other govc commands will auto login when given credentials.
The session.login command can be used to:
- Persist a session without writing to disk via the '-cookie' flag
- Acquire a clone ticket
- Login using a clone ticket
- Avoid passing credentials to other govc commands

Examples:
  govc session.login -u root:password@host
  ticket=$(govc session.login -u root@host -clone)
  govc session.login -u root@host -ticket $ticket`
}

type ticketResult struct {
	cmd    *login
	Ticket string `json:",omitempty"`
	Cookie string `json:",omitempty"`
}

func (r *ticketResult) Write(w io.Writer) error {
	var output []string

	for _, val := range []string{r.Ticket, r.Cookie} {
		if val != "" {
			output = append(output, val)
		}
	}

	if len(output) == 0 {
		return nil
	}

	fmt.Fprintln(w, strings.Join(output, " "))

	return nil
}

// Logout is called by cli.Run()
// We override ClientFlag's Logout impl to avoid ending a session when -persist-session=false,
// otherwise Logout would invalidate the cookie and/or ticket.
func (cmd *login) Logout(ctx context.Context) error {
	if cmd.long || cmd.clone {
		return nil
	}
	return cmd.ClientFlag.Logout(ctx)
}

func (cmd *login) cloneSession(ctx context.Context, c *vim25.Client) error {
	return session.NewManager(c).CloneSession(ctx, cmd.ticket)
}

func (cmd *login) setCookie(ctx context.Context, c *vim25.Client) error {
	url := c.URL()
	jar := c.Client.Jar
	cookies := jar.Cookies(url)
	add := true

	cookie := &http.Cookie{
		Name: soap.SessionCookieName,
	}

	for _, e := range cookies {
		if e.Name == cookie.Name {
			add = false
			cookie = e
			break
		}
	}

	if cmd.cookie == "" {
		// This is the cookie from Set-Cookie after a Login or CloneSession
		cmd.cookie = cookie.Value
	} else {
		// The cookie flag is set, set the HTTP header and skip Login()
		cookie.Value = cmd.cookie
		if add {
			cookies = append(cookies, cookie)
		}
		jar.SetCookies(url, cookies)

		// Check the session is still valid
		_, err := methods.GetCurrentTime(ctx, c)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cmd *login) Run(ctx context.Context, f *flag.FlagSet) error {
	if cmd.ticket != "" {
		cmd.Login = cmd.cloneSession
	} else if cmd.cookie != "" {
		cmd.Login = cmd.setCookie
	}

	c, err := cmd.Client()
	if err != nil {
		return err
	}

	m := session.NewManager(c)
	r := &ticketResult{cmd: cmd}

	if cmd.clone {
		r.Ticket, err = m.AcquireCloneTicket(ctx)
		if err != nil {
			return err
		}
	}

	if cmd.cookie == "" {
		_ = cmd.setCookie(ctx, c)
		if cmd.cookie == "" {
			return flag.ErrHelp
		}
	}

	if cmd.long {
		r.Cookie = cmd.cookie
	}

	return cmd.WriteResult(r)
}
