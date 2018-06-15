#!/usr/bin/env bats

load test_helper

@test "session.ls" {
  esx_env

  run govc session.ls
  assert_success

  run govc session.ls -json
  assert_success

  # Test User-Agent
  govc session.ls | grep "$(govc version | tr ' ' /)"
}

@test "session.rm" {
  esx_env

  run govc session.rm enoent
  assert_failure
  assert_output "govc: ServerFaultCode: The object or item referred to could not be found."

  # Can't remove the current session
  id=$(govc session.ls -json | jq -r .CurrentSession.Key)
  run govc session.rm "$id"
  assert_failure

  thumbprint=$(govc about.cert -thumbprint)
  # persist session just to avoid the Logout() so we can session.rm below
  dir=$(mktemp -d govc-test-XXXXX)

  id=$(GOVMOMI_HOME="$dir" govc session.ls -json -k=false -persist-session -tls-known-hosts <(echo "$thumbprint") | jq -r .CurrentSession.Key)

  rm -rf "$dir"

  run govc session.rm "$id"
  assert_success
}

@test "session.login" {
    esx_env

    # Remove username/password
    host=$(govc env GOVC_URL)

    # Validate auth is not required for service content
    run govc about -u "$host"
    assert_success

    # Auth is required here
    run govc ls -u "$host"
    assert_failure

    cookie=$(govc session.login -l)
    ticket=$(govc session.login -cookie "$cookie" -clone)

    run govc session.login -u "$host" -ticket "$ticket"
    assert_success
}
