//Deprecated: in favor of universe.dagger.io/alpha package
package terraform

// Subcommands of `terraform workspace` command
#WorkspaceSubcmd: "new" | "select" | "delete"

// Run `terraform workspace`
#Workspace: #Run & {
	// Terraform `workspace` command
	cmd: "workspace \(subCmd)"

	// Terraform `workspace` subcommand (i.e. new, select, delete)
	subCmd: #WorkspaceSubcmd
}
