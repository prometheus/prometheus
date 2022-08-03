setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "aws/cli" {
    dagger "do" -p ./sts_get_caller_identity.cue getCallerIdentity
}
