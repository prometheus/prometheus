package marathon

import (
	"fmt"
	"math/rand"
)

const appListPath string = "/v2/apps/?embed=apps.tasks"

// RandomAppsURL randomly selects a server from an array and creates an URL pointing to the app list.
func RandomAppsURL(servers []string) string {
	// TODO If possible update server list from Marathon at some point
	server := servers[rand.Intn(len(servers))]
	return fmt.Sprintf("%s%s", server, appListPath)
}
