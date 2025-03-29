#!/usr/bin/env bash

## Used in the CI to set up size-restricted filesystems for tests.

for i in $(seq 0 99); do
    IMG_FILE="loopfs_${i}.img"
    # tests rely on this mount point, update them accordingly.
    MOUNT_POINT="/mnt/test_prom_size_restricted_fs/fs_${i}"
    truncate --size=8M "${IMG_FILE}"
    LOOP_DEVICE=$(sudo losetup --find --show "${IMG_FILE}")
    sudo mkfs.ext4 -F "${LOOP_DEVICE}" > /dev/null
    sudo mkdir -p "${MOUNT_POINT}"
    sudo mount "${LOOP_DEVICE}" "${MOUNT_POINT}"
    sudo chown $(whoami):$(whoami) "${MOUNT_POINT}"
done
