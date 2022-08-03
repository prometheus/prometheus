setup() {
    load '../../bats_helpers'

    common_setup
}
@test "alpine" {
    dagger "do" -p ./test.cue test
}
