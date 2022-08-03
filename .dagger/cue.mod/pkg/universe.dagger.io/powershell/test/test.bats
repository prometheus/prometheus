setup() {
    load '../../bats_helpers'

    common_setup
}

@test "powershell" {
    dagger "do" -p ./test.cue test
}

