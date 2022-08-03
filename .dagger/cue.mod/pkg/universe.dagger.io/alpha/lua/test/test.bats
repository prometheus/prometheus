setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "lua" {
    dagger "do" -p ./fmtCheck.cue test
}
