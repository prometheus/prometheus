setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "docker/cli" {
    dagger "do" -p ./run.cue test
    dagger "do" -p ./load.cue test
}
