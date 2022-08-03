setup() {
    load '../../../../bats_helpers'

    common_setup
}
@test "apt" {
    dagger "do" -p ./test.cue test
}
