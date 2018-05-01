package base

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// Exec execute a WarpScript on the backend, returning resultat as byte array
func (c *Client) Exec(warpScript string) (body []byte, err error) {
	r := strings.NewReader(warpScript)

	req, err := http.NewRequest("POST", c.Host+c.ExecPath, r)
	if err != nil {
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(bts))
	}

	return
}

// Find execute a WarpScript on the backend, returning resultat as byte array
// Enhance parse response
func (c *Client) Find(selector Selector) (body []byte, err error) {
	req, err := http.NewRequest("GET", c.Host+c.FindPath, nil)
	if err != nil {
		return
	}

	if err = needReadAccess(req, c); err != nil {
		return
	}

	q := req.URL.Query()
	q.Add("selector", string(selector))
	q.Add("format", "fulltext")
	req.URL.RawQuery = q.Encode()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(bts))
	}

	return
}

// Update execute a WarpScript on the backend, returning resultat as byte array
func (c *Client) Update(gts GTSList) (err error) {
	r := strings.NewReader(gts.Sensision())

	req, err := http.NewRequest("POST", c.Host+c.UpdatePath, r)
	if err != nil {
		return
	}

	if err = needWriteAccess(req, c); err != nil {
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return errors.New(string(bts))
	}

	return
}

// Meta execute a WarpScript on the backend, returning resultat as byte array
// Enhance parse response
func (c *Client) Meta(gtsList GTSList) (body []byte, err error) {
	r := strings.NewReader(gtsList.SensisionSelectors(true))
	req, err := http.NewRequest("POST", c.Host+c.MetaPath, r)
	if err != nil {
		return
	}

	if err = needWriteAccess(req, c); err != nil {
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(bts))
	}

	return
}

// Fetch execute a WarpScript on the backend, returning resultat as byte array
// Enhance parse response
func (c *Client) Fetch(selector Selector, start time.Time, stop time.Time) (body []byte, err error) {
	req, err := http.NewRequest("GET", c.Host+c.FetchPath, nil)
	if err != nil {
		return
	}

	if err = needReadAccess(req, c); err != nil {
		return
	}

	q := req.URL.Query()
	q.Add("selector", string(selector))
	q.Add("format", "fulltext")
	q.Add("start", start.UTC().Format(time.RFC3339Nano))
	q.Add("stop", stop.UTC().Format(time.RFC3339Nano))
	req.URL.RawQuery = q.Encode()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(bts))
	}

	return
}

// Delete execute a WarpScript on the backend, returning resultat as byte array
// if start and end are nil, assume user want to delete all datapoints
// Enhance parse response
func (c *Client) Delete(selector Selector, start time.Time, stop time.Time) (body []byte, err error) {
	req, err := http.NewRequest("GET", c.Host+c.DeletePath, nil)
	if err != nil {
		return
	}

	if err = needWriteAccess(req, c); err != nil {
		return
	}

	q := req.URL.Query()
	q.Add("selector", string(selector))
	if start.IsZero() && stop.IsZero() {
		q.Add("deleteall", "")
	} else {
		q.Add("start", start.UTC().Format(time.RFC3339Nano))
		q.Add("stop", stop.UTC().Format(time.RFC3339Nano))
	}
	req.URL.RawQuery = q.Encode()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	bts, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(string(bts))
	}

	return
}

func needReadAccess(req *http.Request, c *Client) error {
	if c.ReadToken == "" {
		return errors.New("This Warp10 call need a READ token access on the data")
	}
	req.Header.Add(c.Warp10Header, c.ReadToken)
	return nil
}

func needWriteAccess(req *http.Request, c *Client) error {
	if c.WriteToken == "" {
		return errors.New("This Warp10 call need a WRITE token access on the data")
	}
	req.Header.Add(c.Warp10Header, c.WriteToken)
	return nil
}
