setup() {
    load '../../bats_helpers'

    common_setup
}

@test "aws default_version" {
    dagger "do" -p ./default_version.cue getVersion
}

@test "aws credentials" {
    dagger "do" -p ./credentials.cue getCallerIdentity
}

@test "aws config_file" {
    dagger "do" -p ./config_file.cue getCallerIdentity
}
