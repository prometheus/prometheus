package slb

import "testing"

func testBackendServers(t *testing.T, client *Client, loadBalancerId string) {

	backendServers := []BackendServerType{
		BackendServerType{
			ServerId: TestInstanceId,
			Weight:   100,
		},
	}

	servers, err := client.AddBackendServers(loadBalancerId, backendServers)

	if err != nil {
		t.Errorf("Failed to AddBackendServers: %v", err)
	}

	t.Logf("Backend servers: %++v", servers)

	backendServers[0].Weight = 80

	servers, err = client.SetBackendServers(loadBalancerId, backendServers)

	if err != nil {
		t.Errorf("Failed to SetBackendServers: %v", err)
	}

	t.Logf("Backend servers: %++v", servers)

	servers, err = client.RemoveBackendServers(loadBalancerId, []string{TestInstanceId})
	if err != nil {
		t.Errorf("Failed to RemoveBackendServers: %v", err)
	}
	t.Logf("Backend servers: %++v", servers)

}
