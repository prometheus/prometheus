setup() {
    load '../../bats_helpers'

    common_setup
}

@test "docker build" {
    dagger "do" -p ./build.cue test
}

@test "docker dockerfile" {
    dagger "do" -p ./dockerfile.cue test
}

@test "docker run" {
    dagger "do" -p ./run.cue test
}

@test "docker image" {
    dagger "do" -p ./image.cue test
}
