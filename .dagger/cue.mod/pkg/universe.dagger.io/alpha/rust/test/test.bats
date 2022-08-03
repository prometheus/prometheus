setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "rust publish" {
    dagger "do" -p ./publish.cue test
}

@test "rust image" {
    dagger "do" -p ./image.cue test
}
