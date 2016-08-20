mdns
====

Simple mDNS client/server library in Golang. mDNS or Multicast DNS can be
used to discover services on the local network without the use of an authoritative
DNS server. This enables peer-to-peer discovery. It is important to note that many
networks restrict the use of multicasting, which prevents mDNS from functioning.
Notably, multicast cannot be used in any sort of cloud, or shared infrastructure
environment. However it works well in most office, home, or private infrastructure
environments.

Using the library is very simple, here is an example of publishing a service entry:

    // Setup our service export
    host, _ := os.Hostname()
    info := []string{"My awesome service"},
    service, _ := NewMDNSService(host, "_foobar._tcp", "", "", 8000, nil, info)

    // Create the mDNS server, defer shutdown
    server, _ := mdns.NewServer(&mdns.Config{Zone: service})
    defer server.Shutdown()


Doing a lookup for service providers is also very simple:

    // Make a channel for results and start listening
    entriesCh := make(chan *mdns.ServiceEntry, 4)
    go func() {
        for entry := range entriesCh {
            fmt.Printf("Got new entry: %v\n", entry)
        }
    }()

    // Start the lookup
    mdns.Lookup("_foobar._tcp", entriesCh)
    close(entriesCh)

