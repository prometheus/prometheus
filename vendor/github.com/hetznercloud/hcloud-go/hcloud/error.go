package hcloud

import "fmt"

// ErrorCode represents an error code returned from the API.
type ErrorCode string

// Error codes returned from the API.
const (
	ErrorCodeServiceError          ErrorCode = "service_error"           // Generic service error
	ErrorCodeRateLimitExceeded     ErrorCode = "rate_limit_exceeded"     // Rate limit exceeded
	ErrorCodeUnknownError          ErrorCode = "unknown_error"           // Unknown error
	ErrorCodeNotFound              ErrorCode = "not_found"               // Resource not found
	ErrorCodeInvalidInput          ErrorCode = "invalid_input"           // Validation error
	ErrorCodeForbidden             ErrorCode = "forbidden"               // Insufficient permissions
	ErrorCodeJSONError             ErrorCode = "json_error"              // Invalid JSON in request
	ErrorCodeLocked                ErrorCode = "locked"                  // Item is locked (Another action is running)
	ErrorCodeResourceLimitExceeded ErrorCode = "resource_limit_exceeded" // Resource limit exceeded
	ErrorCodeResourceUnavailable   ErrorCode = "resource_unavailable"    // Resource currently unavailable
	ErrorCodeUniquenessError       ErrorCode = "uniqueness_error"        // One or more fields must be unique
	ErrorCodeProtected             ErrorCode = "protected"               // The actions you are trying is protected
	ErrorCodeMaintenance           ErrorCode = "maintenance"             // Cannot perform operation due to maintenance
	ErrorCodeConflict              ErrorCode = "conflict"                // The resource has changed during the request, please retry
	ErrorCodeRobotUnavailable      ErrorCode = "robot_unavailable"       // Robot was not available. The caller may retry the operation after a short delay
	ErrorUnsupportedError          ErrorCode = "unsupported_error"       // The gives resource does not support this

	// Server related error codes
	ErrorCodeInvalidServerType     ErrorCode = "invalid_server_type"     // The server type does not fit for the given server or is deprecated
	ErrorCodeServerNotStopped      ErrorCode = "server_not_stopped"      // The action requires a stopped server
	ErrorCodeNetworksOverlap       ErrorCode = "networks_overlap"        // The network IP range overlaps with one of the server networks
	ErrorCodePlacementError        ErrorCode = "placement_error"         // An error during the placement occurred
	ErrorCodeServerAlreadyAttached ErrorCode = "server_already_attached" // The server is already attached to the resource

	// Load Balancer related error codes
	ErrorCodeIPNotOwned                       ErrorCode = "ip_not_owned"                          // The IP you are trying to add as a target is not owned by the Project owner
	ErrorCodeSourcePortAlreadyUsed            ErrorCode = "source_port_already_used"              // The source port you are trying to add is already in use
	ErrorCodeCloudResourceIPNotAllowed        ErrorCode = "cloud_resource_ip_not_allowed"         // The IP you are trying to add as a target belongs to a Hetzner Cloud resource
	ErrorCodeServerNotAttachedToNetwork       ErrorCode = "server_not_attached_to_network"        // The server you are trying to add as a target is not attached to the same network as the Load Balancer
	ErrorCodeTargetAlreadyDefined             ErrorCode = "target_already_defined"                // The Load Balancer target you are trying to define is already defined
	ErrorCodeInvalidLoadBalancerType          ErrorCode = "invalid_load_balancer_type"            // The Load Balancer type does not fit for the given Load Balancer
	ErrorCodeLoadBalancerAlreadyAttached      ErrorCode = "load_balancer_already_attached"        // The Load Balancer is already attached to a network
	ErrorCodeTargetsWithoutUsePrivateIP       ErrorCode = "targets_without_use_private_ip"        // The Load Balancer has targets that use the public IP instead of the private IP
	ErrorCodeLoadBalancerNotAttachedToNetwork ErrorCode = "load_balancer_not_attached_to_network" // The Load Balancer is not attached to a network

	// Network related error codes
	ErrorCodeIPNotAvailable     ErrorCode = "ip_not_available"        // The provided Network IP is not available
	ErrorCodeNoSubnetAvailable  ErrorCode = "no_subnet_available"     // No Subnet or IP is available for the Load Balancer/Server within the network
	ErrorCodeVSwitchAlreadyUsed ErrorCode = "vswitch_id_already_used" // The given Robot vSwitch ID is already registered in another network

	// Volume related error codes
	ErrorCodeNoSpaceLeftInLocation ErrorCode = "no_space_left_in_location" // There is no volume space left in the given location
	ErrorCodeVolumeAlreadyAttached ErrorCode = "volume_already_attached"   // Volume is already attached to a server, detach first

	// Deprecated error codes
	// The actual value of this error code is limit_reached. The new error code
	// rate_limit_exceeded for ratelimiting was introduced before Hetzner Cloud
	// launched into the public. To make clients using the old error code still
	// work as expected, we set the value of the old error code to that of the
	// new error code.
	ErrorCodeLimitReached = ErrorCodeRateLimitExceeded
)

// Error is an error returned from the API.
type Error struct {
	Code    ErrorCode
	Message string
	Details interface{}
}

func (e Error) Error() string {
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// ErrorDetailsInvalidInput contains the details of an 'invalid_input' error.
type ErrorDetailsInvalidInput struct {
	Fields []ErrorDetailsInvalidInputField
}

// ErrorDetailsInvalidInputField contains the validation errors reported on a field.
type ErrorDetailsInvalidInputField struct {
	Name     string
	Messages []string
}

// IsError returns whether err is an API error with the given error code.
func IsError(err error, code ErrorCode) bool {
	apiErr, ok := err.(Error)
	return ok && apiErr.Code == code
}
