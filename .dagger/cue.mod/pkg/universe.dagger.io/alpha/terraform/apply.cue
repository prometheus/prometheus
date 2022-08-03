package terraform

// Run `terraform apply`
#Apply: #Run & {
	// Terraform `apply` command
	cmd: "apply"

	// Internal pre-defined arguments for `terraform apply`
	withinCmdArgs: [
		if autoApprove {
			"-auto-approve"
		},
		planFile,
	]

	// Flag to indicate whether or not to auto-approve (i.e. -auto-approve flag)
	autoApprove: bool | *true

	// Path to a Terraform plan file
	planFile: string | *_#DefaultPlanFile
}
