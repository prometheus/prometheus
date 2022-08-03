setup() {
    load '../../bats_helpers'

    common_setup
}

@test "go build" {
    dagger "do" -p ./build.cue test
}

@test "go container" {
    dagger "do" -p ./container.cue test
}

@test "go image" {
    dagger "do" -p ./image.cue test
}

@test "go test" {
    dagger "do" -p ./test.cue test
}
