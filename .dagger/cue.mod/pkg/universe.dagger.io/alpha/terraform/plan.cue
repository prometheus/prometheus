package terraform

// File to write the output plan
_#DefaultPlanFile: "./out.tfplan"

// Run `terraform plan`
#Plan: #Run & {
	// Terraform `plan` command
	cmd: "plan"

	// Internal pre-defined arguments for `terraform plan`
	withinCmdArgs: ["-out=\(planFile)"]

	// Path to a Terraform plan file
	planFile: string | *_#DefaultPlanFile
}
