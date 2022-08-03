setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "terraform" {
    dagger "do" test
}
