setup() {
    load '../../../../../bats_helpers'

    common_setup
}

@test "cdk python image" {
    dagger "do" -p ./image.cue test
}
