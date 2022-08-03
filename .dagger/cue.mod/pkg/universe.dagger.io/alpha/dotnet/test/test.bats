setup() {
    load '../../../bats_helpers'

    common_setup
}

@test "dotnet publish" {
    dagger "do" -p ./publish.cue test
}

@test "dotnet image" {
    dagger "do" -p ./image.cue test
}

@test "dotnet test" {
    dagger "do" -p ./test.cue test
}
