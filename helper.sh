# #!/bin/bash

# hdiutil create -size 600m -fs HFS+ -volname TestDisk /Users/ntakashi/Workspace/github.com/nicolastakashi/tiny_disk.dmg
# hdiutil attach /Users/ntakashi/Workspace/github.com/nicolastakashi/tiny_disk.dmg -mountpoint /Users/ntakashi/Workspace/github.com/nicolastakashi/prometheus/data
# hdiutil detach ./data/ && rm -rf /Users/ntakashi/Workspace/github.com/nicolastakashi/tiny_disk.dmg

dd if=/dev/zero of=/Users/ntakashi/Workspace/github.com/nicolastakashi/prometheus/data bs=1M count=10
dd if=/dev/zero of=/Users/ntakashi/Workspace/github.com/nicolastakashi/prometheus/data/largefile bs=1M count=585


586Mi
#!/bin/bash

DISK_IMAGE="/Users/ntakashi/Workspace/github.com/nicolastakashi/tiny_disk.dmg"
MOUNT_POINT="/Users/ntakashi/Workspace/github.com/nicolastakashi/prometheus/data"
NEW_SIZE_MB=25 # Target size in MiB

# Step 1: Attach the disk image
echo "Attaching the disk image..."
hdiutil attach $DISK_IMAGE -mountpoint $MOUNT_POINT

# Step 2: Verify used space
USED_SPACE_MB=$(df -m $MOUNT_POINT | awk 'NR==2 {print $3}')
if [ $USED_SPACE_MB -gt $NEW_SIZE_MB ]; then
  echo "Error: Used space ($USED_SPACE_MB MiB) exceeds the target size ($NEW_SIZE_MB MiB)."
  hdiutil detach $MOUNT_POINT
  exit 1
fi

# Step 3: Resize the filesystem
echo "Resizing the filesystem to ${NEW_SIZE_MB} MiB..."
diskutil resizeVolume $MOUNT_POINT ${NEW_SIZE_MB}M

# Step 4: Detach the disk image
echo "Detaching the disk image..."
hdiutil detach $MOUNT_POINT

# Step 5: Shrink the disk image file
echo "Resizing the disk image to ${NEW_SIZE_MB} MiB..."
hdiutil resize -size ${NEW_SIZE_MB}m $DISK_IMAGE

# Step 6: Verify the new size
echo "Verifying the new size..."
hdiutil attach $DISK_IMAGE -mountpoint $MOUNT_POINT
df -h $MOUNT_POINT
hdiutil detach $MOUNT_POINT

echo "Done!"