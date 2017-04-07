package sockaddr

// ifAddrAttrMap is a map of the IfAddr type-specific attributes.
var ifAddrAttrMap map[AttrName]func(IfAddr) string
var ifAddrAttrs []AttrName

func init() {
	ifAddrAttrInit()
}

// GetPrivateIP returns a string with a single IP address that is part of RFC
// 6890 and has a default route.  If the system can't determine its IP address
// or find an RFC 6890 IP address, an empty string will be returned instead.
// This function is the `eval` equivalent of:
//
// ```
// $ sockaddr eval -r '{{GetPrivateInterfaces | attr "address"}}'
/// ```
func GetPrivateIP() (string, error) {
	privateIfs, err := GetPrivateInterfaces()
	if err != nil {
		return "", err
	}
	if len(privateIfs) < 1 {
		return "", nil
	}

	ifAddr := privateIfs[0]
	ip := *ToIPAddr(ifAddr.SockAddr)
	return ip.NetIP().String(), nil
}

// GetPublicIP returns a string with a single IP address that is NOT part of RFC
// 6890 and has a default route.  If the system can't determine its IP address
// or find a non RFC 6890 IP address, an empty string will be returned instead.
// This function is the `eval` equivalent of:
//
// ```
// $ sockaddr eval -r '{{GetPublicInterfaces | attr "address"}}'
/// ```
func GetPublicIP() (string, error) {
	publicIfs, err := GetPublicInterfaces()
	if err != nil {
		return "", err
	} else if len(publicIfs) < 1 {
		return "", nil
	}

	ifAddr := publicIfs[0]
	ip := *ToIPAddr(ifAddr.SockAddr)
	return ip.NetIP().String(), nil
}

// GetInterfaceIP returns a string with a single IP address sorted by the size
// of the network (i.e. IP addresses with a smaller netmask, larger network
// size, are sorted first).  This function is the `eval` equivalent of:
//
// ```
// $ sockaddr eval -r '{{GetAllInterfaces | include "name" <<ARG>> | sort "type,size" | include "flag" "forwardable" | attr "address" }}'
/// ```
func GetInterfaceIP(namedIfRE string) (string, error) {
	ifAddrs, err := GetAllInterfaces()
	if err != nil {
		return "", err
	}

	ifAddrs, _, err = IfByName(namedIfRE, ifAddrs)
	if err != nil {
		return "", err
	}

	ifAddrs, _, err = IfByFlag("forwardable", ifAddrs)
	if err != nil {
		return "", err
	}

	ifAddrs, err = SortIfBy("+type,+size", ifAddrs)
	if err != nil {
		return "", err
	}

	if len(ifAddrs) == 0 {
		return "", err
	}

	ip := ToIPAddr(ifAddrs[0].SockAddr)
	if ip == nil {
		return "", err
	}

	return IPAddrAttr(*ip, "address"), nil
}

// IfAddrAttrs returns a list of attributes supported by the IfAddr type
func IfAddrAttrs() []AttrName {
	return ifAddrAttrs
}

// IfAddrAttr returns a string representation of an attribute for the given
// IfAddr.
func IfAddrAttr(ifAddr IfAddr, attrName AttrName) string {
	fn, found := ifAddrAttrMap[attrName]
	if !found {
		return ""
	}

	return fn(ifAddr)
}

// ifAddrAttrInit is called once at init()
func ifAddrAttrInit() {
	// Sorted for human readability
	ifAddrAttrs = []AttrName{
		"flags",
		"name",
	}

	ifAddrAttrMap = map[AttrName]func(ifAddr IfAddr) string{
		"flags": func(ifAddr IfAddr) string {
			return ifAddr.Interface.Flags.String()
		},
		"name": func(ifAddr IfAddr) string {
			return ifAddr.Interface.Name
		},
	}
}
