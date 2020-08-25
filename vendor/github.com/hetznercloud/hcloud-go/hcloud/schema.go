package hcloud

import (
	"net"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// This file provides converter functions to convert models in the
// schema package to models in the hcloud package and vice versa.

// ActionFromSchema converts a schema.Action to an Action.
func ActionFromSchema(s schema.Action) *Action {
	action := &Action{
		ID:        s.ID,
		Status:    ActionStatus(s.Status),
		Command:   s.Command,
		Progress:  s.Progress,
		Started:   s.Started,
		Resources: []*ActionResource{},
	}
	if s.Finished != nil {
		action.Finished = *s.Finished
	}
	if s.Error != nil {
		action.ErrorCode = s.Error.Code
		action.ErrorMessage = s.Error.Message
	}
	for _, r := range s.Resources {
		action.Resources = append(action.Resources, &ActionResource{
			ID:   r.ID,
			Type: ActionResourceType(r.Type),
		})
	}
	return action
}

// ActionsFromSchema converts a slice of schema.Action to a slice of Action.
func ActionsFromSchema(s []schema.Action) []*Action {
	var actions []*Action
	for _, a := range s {
		actions = append(actions, ActionFromSchema(a))
	}
	return actions
}

// FloatingIPFromSchema converts a schema.FloatingIP to a FloatingIP.
func FloatingIPFromSchema(s schema.FloatingIP) *FloatingIP {
	f := &FloatingIP{
		ID:           s.ID,
		Type:         FloatingIPType(s.Type),
		HomeLocation: LocationFromSchema(s.HomeLocation),
		Created:      s.Created,
		Blocked:      s.Blocked,
		Protection: FloatingIPProtection{
			Delete: s.Protection.Delete,
		},
		Name: s.Name,
	}
	if s.Description != nil {
		f.Description = *s.Description
	}
	if s.Server != nil {
		f.Server = &Server{ID: *s.Server}
	}
	if f.Type == FloatingIPTypeIPv4 {
		f.IP = net.ParseIP(s.IP)
	} else {
		f.IP, f.Network, _ = net.ParseCIDR(s.IP)
	}
	f.DNSPtr = map[string]string{}
	for _, entry := range s.DNSPtr {
		f.DNSPtr[entry.IP] = entry.DNSPtr
	}
	f.Labels = map[string]string{}
	for key, value := range s.Labels {
		f.Labels[key] = value
	}
	return f
}

// ISOFromSchema converts a schema.ISO to an ISO.
func ISOFromSchema(s schema.ISO) *ISO {
	return &ISO{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Type:        ISOType(s.Type),
		Deprecated:  s.Deprecated,
	}
}

// LocationFromSchema converts a schema.Location to a Location.
func LocationFromSchema(s schema.Location) *Location {
	return &Location{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Country:     s.Country,
		City:        s.City,
		Latitude:    s.Latitude,
		Longitude:   s.Longitude,
		NetworkZone: NetworkZone(s.NetworkZone),
	}
}

// DatacenterFromSchema converts a schema.Datacenter to a Datacenter.
func DatacenterFromSchema(s schema.Datacenter) *Datacenter {
	d := &Datacenter{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Location:    LocationFromSchema(s.Location),
		ServerTypes: DatacenterServerTypes{
			Available: []*ServerType{},
			Supported: []*ServerType{},
		},
	}
	for _, t := range s.ServerTypes.Available {
		d.ServerTypes.Available = append(d.ServerTypes.Available, &ServerType{ID: t})
	}
	for _, t := range s.ServerTypes.Supported {
		d.ServerTypes.Supported = append(d.ServerTypes.Supported, &ServerType{ID: t})
	}
	return d
}

// ServerFromSchema converts a schema.Server to a Server.
func ServerFromSchema(s schema.Server) *Server {
	server := &Server{
		ID:              s.ID,
		Name:            s.Name,
		Status:          ServerStatus(s.Status),
		Created:         s.Created,
		PublicNet:       ServerPublicNetFromSchema(s.PublicNet),
		ServerType:      ServerTypeFromSchema(s.ServerType),
		IncludedTraffic: s.IncludedTraffic,
		RescueEnabled:   s.RescueEnabled,
		Datacenter:      DatacenterFromSchema(s.Datacenter),
		Locked:          s.Locked,
		Protection: ServerProtection{
			Delete:  s.Protection.Delete,
			Rebuild: s.Protection.Rebuild,
		},
	}
	if s.Image != nil {
		server.Image = ImageFromSchema(*s.Image)
	}
	if s.BackupWindow != nil {
		server.BackupWindow = *s.BackupWindow
	}
	if s.OutgoingTraffic != nil {
		server.OutgoingTraffic = *s.OutgoingTraffic
	}
	if s.IngoingTraffic != nil {
		server.IngoingTraffic = *s.IngoingTraffic
	}
	if s.ISO != nil {
		server.ISO = ISOFromSchema(*s.ISO)
	}
	server.Labels = map[string]string{}
	for key, value := range s.Labels {
		server.Labels[key] = value
	}
	for _, id := range s.Volumes {
		server.Volumes = append(server.Volumes, &Volume{ID: id})
	}
	for _, privNet := range s.PrivateNet {
		server.PrivateNet = append(server.PrivateNet, ServerPrivateNetFromSchema(privNet))
	}
	return server
}

// ServerPublicNetFromSchema converts a schema.ServerPublicNet to a ServerPublicNet.
func ServerPublicNetFromSchema(s schema.ServerPublicNet) ServerPublicNet {
	publicNet := ServerPublicNet{
		IPv4: ServerPublicNetIPv4FromSchema(s.IPv4),
		IPv6: ServerPublicNetIPv6FromSchema(s.IPv6),
	}
	for _, id := range s.FloatingIPs {
		publicNet.FloatingIPs = append(publicNet.FloatingIPs, &FloatingIP{ID: id})
	}
	return publicNet
}

// ServerPublicNetIPv4FromSchema converts a schema.ServerPublicNetIPv4 to
// a ServerPublicNetIPv4.
func ServerPublicNetIPv4FromSchema(s schema.ServerPublicNetIPv4) ServerPublicNetIPv4 {
	return ServerPublicNetIPv4{
		IP:      net.ParseIP(s.IP),
		Blocked: s.Blocked,
		DNSPtr:  s.DNSPtr,
	}
}

// ServerPublicNetIPv6FromSchema converts a schema.ServerPublicNetIPv6 to
// a ServerPublicNetIPv6.
func ServerPublicNetIPv6FromSchema(s schema.ServerPublicNetIPv6) ServerPublicNetIPv6 {
	ipv6 := ServerPublicNetIPv6{
		Blocked: s.Blocked,
		DNSPtr:  map[string]string{},
	}
	ipv6.IP, ipv6.Network, _ = net.ParseCIDR(s.IP)

	for _, dnsPtr := range s.DNSPtr {
		ipv6.DNSPtr[dnsPtr.IP] = dnsPtr.DNSPtr
	}
	return ipv6
}

// ServerPrivateNetFromSchema converts a schema.ServerPrivateNet to a ServerPrivateNet.
func ServerPrivateNetFromSchema(s schema.ServerPrivateNet) ServerPrivateNet {
	n := ServerPrivateNet{
		Network:    &Network{ID: s.Network},
		IP:         net.ParseIP(s.IP),
		MACAddress: s.MACAddress,
	}
	for _, ip := range s.AliasIPs {
		n.Aliases = append(n.Aliases, net.ParseIP(ip))
	}
	return n
}

// ServerTypeFromSchema converts a schema.ServerType to a ServerType.
func ServerTypeFromSchema(s schema.ServerType) *ServerType {
	st := &ServerType{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Cores:       s.Cores,
		Memory:      s.Memory,
		Disk:        s.Disk,
		StorageType: StorageType(s.StorageType),
		CPUType:     CPUType(s.CPUType),
	}
	for _, price := range s.Prices {
		st.Pricings = append(st.Pricings, ServerTypeLocationPricing{
			Location: &Location{Name: price.Location},
			Hourly: Price{
				Net:   price.PriceHourly.Net,
				Gross: price.PriceHourly.Gross,
			},
			Monthly: Price{
				Net:   price.PriceMonthly.Net,
				Gross: price.PriceMonthly.Gross,
			},
		})
	}
	return st
}

// SSHKeyFromSchema converts a schema.SSHKey to a SSHKey.
func SSHKeyFromSchema(s schema.SSHKey) *SSHKey {
	sshKey := &SSHKey{
		ID:          s.ID,
		Name:        s.Name,
		Fingerprint: s.Fingerprint,
		PublicKey:   s.PublicKey,
		Created:     s.Created,
	}
	sshKey.Labels = map[string]string{}
	for key, value := range s.Labels {
		sshKey.Labels[key] = value
	}
	return sshKey
}

// ImageFromSchema converts a schema.Image to an Image.
func ImageFromSchema(s schema.Image) *Image {
	i := &Image{
		ID:          s.ID,
		Type:        ImageType(s.Type),
		Status:      ImageStatus(s.Status),
		Description: s.Description,
		DiskSize:    s.DiskSize,
		Created:     s.Created,
		RapidDeploy: s.RapidDeploy,
		OSFlavor:    s.OSFlavor,
		Protection: ImageProtection{
			Delete: s.Protection.Delete,
		},
		Deprecated: s.Deprecated,
	}
	if s.Name != nil {
		i.Name = *s.Name
	}
	if s.ImageSize != nil {
		i.ImageSize = *s.ImageSize
	}
	if s.OSVersion != nil {
		i.OSVersion = *s.OSVersion
	}
	if s.CreatedFrom != nil {
		i.CreatedFrom = &Server{
			ID:   s.CreatedFrom.ID,
			Name: s.CreatedFrom.Name,
		}
	}
	if s.BoundTo != nil {
		i.BoundTo = &Server{
			ID: *s.BoundTo,
		}
	}
	i.Labels = map[string]string{}
	for key, value := range s.Labels {
		i.Labels[key] = value
	}
	return i
}

// VolumeFromSchema converts a schema.Volume to a Volume.
func VolumeFromSchema(s schema.Volume) *Volume {
	v := &Volume{
		ID:          s.ID,
		Name:        s.Name,
		Location:    LocationFromSchema(s.Location),
		Size:        s.Size,
		Status:      VolumeStatus(s.Status),
		LinuxDevice: s.LinuxDevice,
		Protection: VolumeProtection{
			Delete: s.Protection.Delete,
		},
		Created: s.Created,
	}
	if s.Server != nil {
		v.Server = &Server{ID: *s.Server}
	}
	v.Labels = map[string]string{}
	for key, value := range s.Labels {
		v.Labels[key] = value
	}
	return v
}

// NetworkFromSchema converts a schema.Network to a Network.
func NetworkFromSchema(s schema.Network) *Network {
	n := &Network{
		ID:      s.ID,
		Name:    s.Name,
		Created: s.Created,
		Protection: NetworkProtection{
			Delete: s.Protection.Delete,
		},
		Labels: map[string]string{},
	}

	_, n.IPRange, _ = net.ParseCIDR(s.IPRange)

	for _, subnet := range s.Subnets {
		n.Subnets = append(n.Subnets, NetworkSubnetFromSchema(subnet))
	}
	for _, route := range s.Routes {
		n.Routes = append(n.Routes, NetworkRouteFromSchema(route))
	}
	for _, serverID := range s.Servers {
		n.Servers = append(n.Servers, &Server{ID: serverID})
	}
	for key, value := range s.Labels {
		n.Labels[key] = value
	}

	return n
}

// NetworkSubnetFromSchema converts a schema.NetworkSubnet to a NetworkSubnet.
func NetworkSubnetFromSchema(s schema.NetworkSubnet) NetworkSubnet {
	sn := NetworkSubnet{
		Type:        NetworkSubnetType(s.Type),
		NetworkZone: NetworkZone(s.NetworkZone),
		Gateway:     net.ParseIP(s.Gateway),
	}
	_, sn.IPRange, _ = net.ParseCIDR(s.IPRange)
	return sn
}

// NetworkRouteFromSchema converts a schema.NetworkRoute to a NetworkRoute.
func NetworkRouteFromSchema(s schema.NetworkRoute) NetworkRoute {
	r := NetworkRoute{
		Gateway: net.ParseIP(s.Gateway),
	}
	_, r.Destination, _ = net.ParseCIDR(s.Destination)
	return r
}

// LoadBalancerTypeFromSchema converts a schema.LoadBalancerType to a LoadBalancerType.
func LoadBalancerTypeFromSchema(s schema.LoadBalancerType) *LoadBalancerType {
	lt := &LoadBalancerType{
		ID:                      s.ID,
		Name:                    s.Name,
		Description:             s.Description,
		MaxConnections:          s.MaxConnections,
		MaxServices:             s.MaxServices,
		MaxTargets:              s.MaxTargets,
		MaxAssignedCertificates: s.MaxAssignedCertificates,
	}
	for _, price := range s.Prices {
		lt.Pricings = append(lt.Pricings, LoadBalancerTypeLocationPricing{
			Location: &Location{Name: price.Location},
			Hourly: Price{
				Net:   price.PriceHourly.Net,
				Gross: price.PriceHourly.Gross,
			},
			Monthly: Price{
				Net:   price.PriceMonthly.Net,
				Gross: price.PriceMonthly.Gross,
			},
		})
	}
	return lt
}

// LoadBalancerFromSchema converts a schema.LoadBalancer to a LoadBalancer.
func LoadBalancerFromSchema(s schema.LoadBalancer) *LoadBalancer {
	l := &LoadBalancer{
		ID:   s.ID,
		Name: s.Name,
		PublicNet: LoadBalancerPublicNet{
			Enabled: s.PublicNet.Enabled,
			IPv4: LoadBalancerPublicNetIPv4{
				IP: net.ParseIP(s.PublicNet.IPv4.IP),
			},
			IPv6: LoadBalancerPublicNetIPv6{
				IP: net.ParseIP(s.PublicNet.IPv6.IP),
			},
		},
		Location:         LocationFromSchema(s.Location),
		LoadBalancerType: LoadBalancerTypeFromSchema(s.LoadBalancerType),
		Algorithm:        LoadBalancerAlgorithm{Type: LoadBalancerAlgorithmType(s.Algorithm.Type)},
		Protection: LoadBalancerProtection{
			Delete: s.Protection.Delete,
		},
		Labels:          map[string]string{},
		Created:         s.Created,
		IncludedTraffic: s.IncludedTraffic,
	}
	for _, privateNet := range s.PrivateNet {
		l.PrivateNet = append(l.PrivateNet, LoadBalancerPrivateNet{
			Network: &Network{ID: privateNet.Network},
			IP:      net.ParseIP(privateNet.IP),
		})
	}
	if s.OutgoingTraffic != nil {
		l.OutgoingTraffic = *s.OutgoingTraffic
	}
	if s.IngoingTraffic != nil {
		l.IngoingTraffic = *s.IngoingTraffic
	}
	for _, service := range s.Services {
		l.Services = append(l.Services, LoadBalancerServiceFromSchema(service))
	}
	for _, target := range s.Targets {
		l.Targets = append(l.Targets, LoadBalancerTargetFromSchema(target))
	}
	for key, value := range s.Labels {
		l.Labels[key] = value
	}
	return l
}

// LoadBalancerServiceFromSchema converts a schema.LoadBalancerService to a LoadBalancerService.
func LoadBalancerServiceFromSchema(s schema.LoadBalancerService) LoadBalancerService {
	ls := LoadBalancerService{
		Protocol:        LoadBalancerServiceProtocol(s.Protocol),
		ListenPort:      s.ListenPort,
		DestinationPort: s.DestinationPort,
		Proxyprotocol:   s.Proxyprotocol,
		HealthCheck:     LoadBalancerServiceHealthCheckFromSchema(s.HealthCheck),
	}
	if s.HTTP != nil {
		ls.HTTP = LoadBalancerServiceHTTP{
			CookieName:     s.HTTP.CookieName,
			CookieLifetime: time.Duration(s.HTTP.CookieLifetime) * time.Second,
			RedirectHTTP:   s.HTTP.RedirectHTTP,
			StickySessions: s.HTTP.StickySessions,
		}
		for _, certificateID := range s.HTTP.Certificates {
			ls.HTTP.Certificates = append(ls.HTTP.Certificates, &Certificate{ID: certificateID})
		}
	}
	return ls
}

// LoadBalancerServiceHealthCheckFromSchema converts a schema.LoadBalancerServiceHealthCheck to a LoadBalancerServiceHealthCheck.
func LoadBalancerServiceHealthCheckFromSchema(s *schema.LoadBalancerServiceHealthCheck) LoadBalancerServiceHealthCheck {
	lsh := LoadBalancerServiceHealthCheck{
		Protocol: LoadBalancerServiceProtocol(s.Protocol),
		Port:     s.Port,
		Interval: time.Duration(s.Interval) * time.Second,
		Retries:  s.Retries,
		Timeout:  time.Duration(s.Timeout) * time.Second,
	}
	if s.HTTP != nil {
		lsh.HTTP = &LoadBalancerServiceHealthCheckHTTP{
			Domain:      s.HTTP.Domain,
			Path:        s.HTTP.Path,
			Response:    s.HTTP.Response,
			StatusCodes: s.HTTP.StatusCodes,
			TLS:         s.HTTP.TLS,
		}
	}
	return lsh
}

// LoadBalancerTargetFromSchema converts a schema.LoadBalancerTarget to a LoadBalancerTarget.
func LoadBalancerTargetFromSchema(s schema.LoadBalancerTarget) LoadBalancerTarget {
	lt := LoadBalancerTarget{
		Type:         LoadBalancerTargetType(s.Type),
		UsePrivateIP: s.UsePrivateIP,
	}
	if s.Server != nil {
		lt.Server = &LoadBalancerTargetServer{
			Server: &Server{ID: s.Server.ID},
		}
	}
	if s.LabelSelector != nil {
		lt.LabelSelector = &LoadBalancerTargetLabelSelector{
			Selector: s.LabelSelector.Selector,
		}
	}
	if s.IP != nil {
		lt.IP = &LoadBalancerTargetIP{IP: s.IP.IP}
	}

	for _, healthStatus := range s.HealthStatus {
		lt.HealthStatus = append(lt.HealthStatus, LoadBalancerTargetHealthStatusFromSchema(healthStatus))
	}
	for _, target := range s.Targets {
		lt.Targets = append(lt.Targets, LoadBalancerTargetFromSchema(target))
	}
	return lt
}

// LoadBalancerTargetHealthStatusFromSchema converts a schema.LoadBalancerTarget to a LoadBalancerTarget.
func LoadBalancerTargetHealthStatusFromSchema(s schema.LoadBalancerTargetHealthStatus) LoadBalancerTargetHealthStatus {
	return LoadBalancerTargetHealthStatus{
		ListenPort: s.ListenPort,
		Status:     LoadBalancerTargetHealthStatusStatus(s.Status),
	}
}

// CertificateFromSchema converts a schema.Certificate to a Certificate.
func CertificateFromSchema(s schema.Certificate) *Certificate {
	c := &Certificate{
		ID:             s.ID,
		Name:           s.Name,
		Certificate:    s.Certificate,
		Created:        s.Created,
		NotValidBefore: s.NotValidBefore,
		NotValidAfter:  s.NotValidAfter,
		DomainNames:    s.DomainNames,
		Fingerprint:    s.Fingerprint,
	}
	if len(s.Labels) > 0 {
		c.Labels = make(map[string]string)
	}
	for key, value := range s.Labels {
		c.Labels[key] = value
	}
	return c
}

// PaginationFromSchema converts a schema.MetaPagination to a Pagination.
func PaginationFromSchema(s schema.MetaPagination) Pagination {
	return Pagination{
		Page:         s.Page,
		PerPage:      s.PerPage,
		PreviousPage: s.PreviousPage,
		NextPage:     s.NextPage,
		LastPage:     s.LastPage,
		TotalEntries: s.TotalEntries,
	}
}

// ErrorFromSchema converts a schema.Error to an Error.
func ErrorFromSchema(s schema.Error) Error {
	e := Error{
		Code:    ErrorCode(s.Code),
		Message: s.Message,
	}

	switch d := s.Details.(type) {
	case schema.ErrorDetailsInvalidInput:
		details := ErrorDetailsInvalidInput{
			Fields: []ErrorDetailsInvalidInputField{},
		}
		for _, field := range d.Fields {
			details.Fields = append(details.Fields, ErrorDetailsInvalidInputField{
				Name:     field.Name,
				Messages: field.Messages,
			})
		}
		e.Details = details
	}
	return e
}

// PricingFromSchema converts a schema.Pricing to a Pricing.
func PricingFromSchema(s schema.Pricing) Pricing {
	p := Pricing{
		Image: ImagePricing{
			PerGBMonth: Price{
				Currency: s.Currency,
				VATRate:  s.VATRate,
				Net:      s.Image.PricePerGBMonth.Net,
				Gross:    s.Image.PricePerGBMonth.Gross,
			},
		},
		FloatingIP: FloatingIPPricing{
			Monthly: Price{
				Currency: s.Currency,
				VATRate:  s.VATRate,
				Net:      s.FloatingIP.PriceMonthly.Net,
				Gross:    s.FloatingIP.PriceMonthly.Gross,
			},
		},
		Traffic: TrafficPricing{
			PerTB: Price{
				Currency: s.Currency,
				VATRate:  s.VATRate,
				Net:      s.Traffic.PricePerTB.Net,
				Gross:    s.Traffic.PricePerTB.Gross,
			},
		},
		ServerBackup: ServerBackupPricing{
			Percentage: s.ServerBackup.Percentage,
		},
	}
	for _, serverType := range s.ServerTypes {
		var pricings []ServerTypeLocationPricing
		for _, price := range serverType.Prices {
			pricings = append(pricings, ServerTypeLocationPricing{
				Location: &Location{Name: price.Location},
				Hourly: Price{
					Currency: s.Currency,
					VATRate:  s.VATRate,
					Net:      price.PriceHourly.Net,
					Gross:    price.PriceHourly.Gross,
				},
				Monthly: Price{
					Currency: s.Currency,
					VATRate:  s.VATRate,
					Net:      price.PriceMonthly.Net,
					Gross:    price.PriceMonthly.Gross,
				},
			})
		}
		p.ServerTypes = append(p.ServerTypes, ServerTypePricing{
			ServerType: &ServerType{
				ID:   serverType.ID,
				Name: serverType.Name,
			},
			Pricings: pricings,
		})
	}
	for _, loadBalancerType := range s.LoadBalancerTypes {
		var pricings []LoadBalancerTypeLocationPricing
		for _, price := range loadBalancerType.Prices {
			pricings = append(pricings, LoadBalancerTypeLocationPricing{
				Location: &Location{Name: price.Location},
				Hourly: Price{
					Currency: s.Currency,
					VATRate:  s.VATRate,
					Net:      price.PriceHourly.Net,
					Gross:    price.PriceHourly.Gross,
				},
				Monthly: Price{
					Currency: s.Currency,
					VATRate:  s.VATRate,
					Net:      price.PriceMonthly.Net,
					Gross:    price.PriceMonthly.Gross,
				},
			})
		}
		p.LoadBalancerTypes = append(p.LoadBalancerTypes, LoadBalancerTypePricing{
			LoadBalancerType: &LoadBalancerType{
				ID:   loadBalancerType.ID,
				Name: loadBalancerType.Name,
			},
			Pricings: pricings,
		})
	}
	return p
}

func loadBalancerCreateOptsToSchema(opts LoadBalancerCreateOpts) schema.LoadBalancerCreateRequest {
	req := schema.LoadBalancerCreateRequest{
		Name:            opts.Name,
		PublicInterface: opts.PublicInterface,
	}
	if opts.Algorithm != nil {
		req.Algorithm = &schema.LoadBalancerCreateRequestAlgorithm{
			Type: string(opts.Algorithm.Type),
		}
	}
	if opts.LoadBalancerType.ID != 0 {
		req.LoadBalancerType = opts.LoadBalancerType.ID
	} else if opts.LoadBalancerType.Name != "" {
		req.LoadBalancerType = opts.LoadBalancerType.Name
	}
	if opts.Location != nil {
		if opts.Location.ID != 0 {
			req.Location = String(strconv.Itoa(opts.Location.ID))
		} else {
			req.Location = String(opts.Location.Name)
		}
	}
	if opts.NetworkZone != "" {
		req.NetworkZone = String(string(opts.NetworkZone))
	}
	if opts.Labels != nil {
		req.Labels = &opts.Labels
	}
	if opts.Network != nil {
		req.Network = Int(opts.Network.ID)
	}
	for _, target := range opts.Targets {
		schemaTarget := schema.LoadBalancerCreateRequestTarget{}
		switch target.Type {
		case LoadBalancerTargetTypeServer:
			schemaTarget.Type = string(LoadBalancerTargetTypeServer)
			schemaTarget.Server = &schema.LoadBalancerCreateRequestTargetServer{ID: target.Server.Server.ID}
		case LoadBalancerTargetTypeLabelSelector:
			schemaTarget.Type = string(LoadBalancerTargetTypeLabelSelector)
			schemaTarget.LabelSelector = &schema.LoadBalancerCreateRequestTargetLabelSelector{Selector: target.LabelSelector.Selector}
		case LoadBalancerTargetTypeIP:
			schemaTarget.Type = string(LoadBalancerTargetTypeIP)
			schemaTarget.IP = &schema.LoadBalancerCreateRequestTargetIP{IP: target.IP.IP}
		}
		req.Targets = append(req.Targets, schemaTarget)
	}
	for _, service := range opts.Services {
		schemaService := schema.LoadBalancerCreateRequestService{
			Protocol:        string(service.Protocol),
			ListenPort:      service.ListenPort,
			DestinationPort: service.DestinationPort,
			Proxyprotocol:   service.Proxyprotocol,
		}
		if service.HTTP != nil {
			schemaService.HTTP = &schema.LoadBalancerCreateRequestServiceHTTP{
				RedirectHTTP:   service.HTTP.RedirectHTTP,
				StickySessions: service.HTTP.StickySessions,
				CookieName:     service.HTTP.CookieName,
			}
			if sec := service.HTTP.CookieLifetime.Seconds(); sec != 0 {
				schemaService.HTTP.CookieLifetime = Int(int(sec))
			}
			if service.HTTP.Certificates != nil {
				certificates := []int{}
				for _, certificate := range service.HTTP.Certificates {
					certificates = append(certificates, certificate.ID)
				}
				schemaService.HTTP.Certificates = &certificates
			}
		}
		if service.HealthCheck != nil {
			schemaHealthCheck := &schema.LoadBalancerCreateRequestServiceHealthCheck{
				Protocol: string(service.HealthCheck.Protocol),
				Port:     service.HealthCheck.Port,
				Retries:  service.HealthCheck.Retries,
			}
			if service.HealthCheck.Interval != nil {
				schemaHealthCheck.Interval = Int(int(service.HealthCheck.Interval.Seconds()))
			}
			if service.HealthCheck.Timeout != nil {
				schemaHealthCheck.Timeout = Int(int(service.HealthCheck.Timeout.Seconds()))
			}
			if service.HealthCheck.HTTP != nil {
				schemaHealthCheckHTTP := &schema.LoadBalancerCreateRequestServiceHealthCheckHTTP{
					Domain:   service.HealthCheck.HTTP.Domain,
					Path:     service.HealthCheck.HTTP.Path,
					Response: service.HealthCheck.HTTP.Response,
					TLS:      service.HealthCheck.HTTP.TLS,
				}
				if service.HealthCheck.HTTP.StatusCodes != nil {
					schemaHealthCheckHTTP.StatusCodes = &service.HealthCheck.HTTP.StatusCodes
				}
				schemaHealthCheck.HTTP = schemaHealthCheckHTTP
			}
			schemaService.HealthCheck = schemaHealthCheck
		}
		req.Services = append(req.Services, schemaService)
	}
	return req
}

func loadBalancerAddServiceOptsToSchema(opts LoadBalancerAddServiceOpts) schema.LoadBalancerActionAddServiceRequest {
	req := schema.LoadBalancerActionAddServiceRequest{
		Protocol:        string(opts.Protocol),
		ListenPort:      opts.ListenPort,
		DestinationPort: opts.DestinationPort,
		Proxyprotocol:   opts.Proxyprotocol,
	}
	if opts.HTTP != nil {
		req.HTTP = &schema.LoadBalancerActionAddServiceRequestHTTP{
			CookieName:     opts.HTTP.CookieName,
			RedirectHTTP:   opts.HTTP.RedirectHTTP,
			StickySessions: opts.HTTP.StickySessions,
		}
		if opts.HTTP.CookieLifetime != nil {
			req.HTTP.CookieLifetime = Int(int(opts.HTTP.CookieLifetime.Seconds()))
		}
		if opts.HTTP.Certificates != nil {
			certificates := []int{}
			for _, certificate := range opts.HTTP.Certificates {
				certificates = append(certificates, certificate.ID)
			}
			req.HTTP.Certificates = &certificates
		}
	}
	if opts.HealthCheck != nil {
		req.HealthCheck = &schema.LoadBalancerActionAddServiceRequestHealthCheck{
			Protocol: string(opts.HealthCheck.Protocol),
			Port:     opts.HealthCheck.Port,
			Retries:  opts.HealthCheck.Retries,
		}
		if opts.HealthCheck.Interval != nil {
			req.HealthCheck.Interval = Int(int(opts.HealthCheck.Interval.Seconds()))
		}
		if opts.HealthCheck.Timeout != nil {
			req.HealthCheck.Timeout = Int(int(opts.HealthCheck.Timeout.Seconds()))
		}
		if opts.HealthCheck.HTTP != nil {
			req.HealthCheck.HTTP = &schema.LoadBalancerActionAddServiceRequestHealthCheckHTTP{
				Domain:   opts.HealthCheck.HTTP.Domain,
				Path:     opts.HealthCheck.HTTP.Path,
				Response: opts.HealthCheck.HTTP.Response,
				TLS:      opts.HealthCheck.HTTP.TLS,
			}
			if opts.HealthCheck.HTTP.StatusCodes != nil {
				req.HealthCheck.HTTP.StatusCodes = &opts.HealthCheck.HTTP.StatusCodes
			}
		}
	}
	return req
}

func loadBalancerUpdateServiceOptsToSchema(opts LoadBalancerUpdateServiceOpts) schema.LoadBalancerActionUpdateServiceRequest {
	req := schema.LoadBalancerActionUpdateServiceRequest{
		DestinationPort: opts.DestinationPort,
		Proxyprotocol:   opts.Proxyprotocol,
	}
	if opts.Protocol != "" {
		req.Protocol = String(string(opts.Protocol))
	}
	if opts.HTTP != nil {
		req.HTTP = &schema.LoadBalancerActionUpdateServiceRequestHTTP{
			CookieName:     opts.HTTP.CookieName,
			RedirectHTTP:   opts.HTTP.RedirectHTTP,
			StickySessions: opts.HTTP.StickySessions,
		}
		if opts.HTTP.CookieLifetime != nil {
			req.HTTP.CookieLifetime = Int(int(opts.HTTP.CookieLifetime.Seconds()))
		}
		if opts.HTTP.Certificates != nil {
			certificates := []int{}
			for _, certificate := range opts.HTTP.Certificates {
				certificates = append(certificates, certificate.ID)
			}
			req.HTTP.Certificates = &certificates
		}
	}
	if opts.HealthCheck != nil {
		req.HealthCheck = &schema.LoadBalancerActionUpdateServiceRequestHealthCheck{
			Port:    opts.HealthCheck.Port,
			Retries: opts.HealthCheck.Retries,
		}
		if opts.HealthCheck.Interval != nil {
			req.HealthCheck.Interval = Int(int(opts.HealthCheck.Interval.Seconds()))
		}
		if opts.HealthCheck.Timeout != nil {
			req.HealthCheck.Timeout = Int(int(opts.HealthCheck.Timeout.Seconds()))
		}
		if opts.HealthCheck.Protocol != "" {
			req.HealthCheck.Protocol = String(string(opts.HealthCheck.Protocol))
		}
		if opts.HealthCheck.HTTP != nil {
			req.HealthCheck.HTTP = &schema.LoadBalancerActionUpdateServiceRequestHealthCheckHTTP{
				Domain:   opts.HealthCheck.HTTP.Domain,
				Path:     opts.HealthCheck.HTTP.Path,
				Response: opts.HealthCheck.HTTP.Response,
				TLS:      opts.HealthCheck.HTTP.TLS,
			}
			if opts.HealthCheck.HTTP.StatusCodes != nil {
				req.HealthCheck.HTTP.StatusCodes = &opts.HealthCheck.HTTP.StatusCodes
			}
		}
	}
	return req
}
