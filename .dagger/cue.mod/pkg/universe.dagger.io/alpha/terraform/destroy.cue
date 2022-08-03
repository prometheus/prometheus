package terraform

// Run `terraform destroy`
#Destroy: #Run & {
	// Terraform `destroy` command
	cmd: "destroy"

	// Internal pre-defined arguments for `terraform destroy`
	withinCmdArgs: [
		if autoApprove {
			"-auto-approve"
		},
	]

	// Flag to indicate whether or not to auto-approve (i.e. -auto-approve flag)
	autoApprove: bool | *true
}
