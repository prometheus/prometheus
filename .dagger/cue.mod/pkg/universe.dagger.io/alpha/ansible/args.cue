package ansible

import (
	"strings"
)

#InventoryArg: {
	inventoryInput: #Inventory
	arg:            "-i inventory/\(inventoryInput.file)"
} | {
	inventoryInput: null
	arg:            ""
}

#ExtraVarsArg: {
	extraVarsInput: [string]
	arg: "--extra-vars \(strings.Join(extraVarsInput, " "))"
} | {
	extraVarsInput: null
	arg:            ""
}

#LogLevelArg: {
	logLevelInput: 0
	arg:           ""
} | {
	logLevelInput: 1
	arg:           "-v"
} | {
	logLevelInput: 2
	arg:           "-vv"
} | {
	logLevelInput: 3
	arg:           "-vvv"
}
