setup() {
    load '../../../../bats_helpers'

    common_setup
}

@test "goreleaser image" {
    dagger "do" -p ./image.cue test
}

@test "goreleaser release" {
    dagger "do" -p ./release.cue test
}
