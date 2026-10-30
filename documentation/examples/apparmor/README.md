# AppArmor Example

`prometheus` is an AppArmor example profile for a conventional Linux
installation:

- Binary: `/usr/bin/prometheus`.
- Configuration and rules: `/etc/prometheus/`.
- TSDB: `/var/lib/prometheus/`.

Copy the profile to `/etc/apparmor.d/usr.bin.prometheus`, review the paths, and
load it with:

```sh
sudo apparmor_parser -r /etc/apparmor.d/usr.bin.prometheus
sudo aa-complain /usr/bin/prometheus
```

Run Prometheus with its normal configuration, review denials, and add only the
access required by the deployment:

```sh
sudo aa-logprof
```

After testing, enforce the profile:

```sh
sudo aa-enforce /usr/bin/prometheus
```

For a locally built binary installed at `/usr/local/bin/prometheus`, change
the profile attachment and use that path in the commands above. Kubernetes,
TLS, service-discovery, remote-write, and custom storage layouts may require
additional read permissions. The profile does not grant access to arbitrary
external programs and does not replace running Prometheus as an unprivileged
user.
