#!/usr/bin/env bats

load test_helper

@test "logs" {
  esx_env

  run govc logs
  assert_success
  nlogs=${#lines[@]}
  # there should be plenty more than 1 line of hostd logs
  [ $nlogs -ge 1 ]

  # test -n flag
  run govc logs -n $((nlogs - 10))
  assert_success
  [ ${#lines[@]} -le $nlogs ]

  run govc logs -log vmkernel
  assert_success
  nlogs=${#lines[@]}
  # there should be plenty more than 1 line of vmkernel logs
  [ $nlogs -ge 1 ]

  # test > 1 call to BrowseLog()
  run govc logs -n 2002
  assert_success

  # -host ignored against ESX
  run govc logs -host enoent
  assert_success

  run govc logs -log enoent
  assert_failure
}

@test "logs.ls" {
  esx_env

  run govc logs.ls
  assert_success

  # -host ignored against ESX
  run govc logs.ls -host enoent
  assert_success
}

@test "logs opid" {
  esx_env

  id=$(new_id)

  run env GOVC_OPERATION_ID="$id" govc events
  assert_success

  govc logs | grep "$id"
}
