package collins

// The ManagementService provides functions to use the IPMI functionality in
// collins, as well as provision servers.
//
// http://tumblr.github.io/collins/api.html#asset
type ManagementService struct {
	client *Client
}

// ProvisionOpts are options that can be passed to colllins when provisioning a
// server.
type ProvisionOpts struct {
	Suffix        string `url:"suffix"`
	PrimaryRole   string `url:"primary_role"`
	SecondaryRole string `url:"secondary_role"`
	Pool          string `url:"pool"`
	Activate      string `url:"activate"`
	// The following are mandatory and set as parameters to Provision()
	Tag     string `url:"tag"`
	Profile string `url:"profile"`
	Contact string `url:"contact"`
}

// powerActions is an internal function for performing various power actions via
// collins.
func (s ManagementService) powerAction(tag, action string) (*Response, error) {
	data := struct {
		Action string `url:"action"`
	}{action}

	ustr, err := addOptions("api/asset/"+tag+"/power", data)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}

// ManagementService.PowerOff powers off the asset, without grace (like pressing
// the power button).
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) PowerOff(tag string) (*Response, error) {
	return s.powerAction(tag, "powerOff")
}

// ManagementService.PowerOn powers up an asset.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) PowerOn(tag string) (*Response, error) {
	return s.powerAction(tag, "powerOn")
}

// Management.SoftPowerOff initiates soft shutdown of OS via ACPI.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) SoftPowerOff(tag string) (*Response, error) {
	return s.powerAction(tag, "powerSoft")
}

// ManagementService.SoftReboot performs a graceful reboot via IPMI, uses ACPI
// to notify OS.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) SoftReboot(tag string) (*Response, error) {
	return s.powerAction(tag, "rebootSoft")
}

// ManagementService.HardReboot is equivalent to pressing the reset button.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) HardReboot(tag string) (*Response, error) {
	return s.powerAction(tag, "rebootHard")
}

// ManagementService.Identify turns on the IPMI light.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) Identify(tag string) (*Response, error) {
	return s.powerAction(tag, "identify")
}

// ManagementService.Verify detects whether the IPMI interface is reachable.
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-managment
func (s ManagementService) Verify(tag string) (*Response, error) {
	return s.powerAction(tag, "verify")
}

// ManagementService.PowerStatus checks the power status of an asset. It returns
// one of the strings "on", "off" or "unknown".
//
// http://tumblr.github.io/collins/api.html#api-asset%20managment-power-status
func (s ManagementService) PowerStatus(tag string) (string, *Response, error) {
	ustr, err := addOptions("api/asset/"+tag+"/power", nil)
	if err != nil {
		return "", nil, err
	}

	req, err := s.client.NewRequest("GET", ustr)
	if err != nil {
		return "", nil, err
	}

	var c struct {
		Message string `json:"MESSAGE"`
	}

	resp, err := s.client.Do(req, &c)
	if err != nil {
		return "", resp, err
	}

	return c.Message, resp, nil
}

// ManagementService.Provision provisions an asset according to the profile and
// options provided.
func (s ManagementService) Provision(tag, profile, contact string, opts ProvisionOpts) (*Response, error) {
	opts.Tag = tag
	opts.Profile = profile
	opts.Contact = contact
	ustr, err := addOptions("api/provision/"+tag, opts)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("POST", ustr)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	return resp, err
}
