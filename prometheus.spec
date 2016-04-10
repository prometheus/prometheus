%define debug_package %{nil}

Name:           prometheus
Version:        %{version}
Release:        1%{?dist}
Summary:        Prometheus is a systems and service monitoring system. It collects metrics from configured targets at given intervals, evaluates rule expressions, displays the results, and can trigger alerts if some condition is observed to be true.
Group:          System Environment/Daemons
License:        See the LICENSE file at github.
URL:            https://github.com/prometheus/prometheus
Requires(pre):  /usr/sbin/useradd
Requires:       systemd-units
AutoReqProv:    No

%description

Prometheus is a systems and service monitoring system.
It collects metrics from configured targets at given intervals, evaluates
rule expressions, displays the results, and can trigger alerts if
some condition is observed to be true.

%prep

%build

%install
install -d \
	var/log/prometheus \
	var/run/prometheus \
	var/lib/prometheus \
	usr/bin \
	lib/systemd/system \
	etc/prometheus \
	etc/sysconfig \
	usr/share/prometheus \
	usr/share/prometheus/consoles \
	usr/share/prometheus/console_libraries

install -m 755 "%{src_root}/rpm/prometheus.service" lib/systemd/system/prometheus.service
install -m 644 "%{src_root}/rpm/prometheus.sysconfig" etc/sysconfig/prometheus
install -m 644 "%{src_root}/rpm/prometheus.yaml" etc/prometheus/prometheus.yaml
install -m 755 "%{src_root}/prometheus" usr/bin/prometheus
install -m 755 "%{src_root}/promtool" usr/bin/promtool

for f in "%{src_root}/consoles"/*.html "%{src_root}/consoles"/index.html.example; do
    install -m 644 "$f" usr/share/prometheus/consoles/
done
for f in "%{src_root}/console_libraries"/*.lib; do
    install -m 644 "$f" usr/share/prometheus/console_libraries/
done

%clean

%pre
getent group prometheus >/dev/null || groupadd -r prometheus
getent passwd prometheus >/dev/null || \
  useradd -r -g prometheus -s /sbin/nologin \
    -d /var/lib/prometheus -c prometheus prometheus
exit 0

%post
chgrp prometheus /var/run/prometheus
chmod 774 /var/run/prometheus
chown prometheus:prometheus /var/log/prometheus
chmod 744 /var/log/prometheus

%files
%defattr(-,root,root,-)
/usr/bin/prometheus
/usr/bin/promtool
%config(noreplace) /etc/prometheus/prometheus.yaml
%config(noreplace) /etc/logrotate.d/prometheus
/lib/systemd/system/prometheus.service
%config(noreplace) /etc/sysconfig/prometheus
/usr/share/prometheus/consoles/aws_elasticache.html
/usr/share/prometheus/consoles/aws_elb.html
/usr/share/prometheus/consoles/aws_redshift-cluster.html
/usr/share/prometheus/consoles/aws_redshift.html
/usr/share/prometheus/consoles/blackbox.html
/usr/share/prometheus/consoles/cassandra.html
/usr/share/prometheus/consoles/cloudwatch.html
/usr/share/prometheus/consoles/haproxy-backend.html
/usr/share/prometheus/consoles/haproxy-backends.html
/usr/share/prometheus/consoles/haproxy-frontend.html
/usr/share/prometheus/consoles/haproxy-frontends.html
/usr/share/prometheus/consoles/haproxy.html
/usr/share/prometheus/consoles/index.html.example
/usr/share/prometheus/consoles/node-cpu.html
/usr/share/prometheus/consoles/node-disk.html
/usr/share/prometheus/consoles/node-overview.html
/usr/share/prometheus/consoles/node.html
/usr/share/prometheus/consoles/prometheus-overview.html
/usr/share/prometheus/consoles/prometheus.html
/usr/share/prometheus/consoles/snmp-overview.html
/usr/share/prometheus/consoles/snmp.html
/usr/share/prometheus/console_libraries/prom.lib
/usr/share/prometheus/console_libraries/menu.lib
%attr(755, prometheus, prometheus)/var/lib/prometheus
/var/run/prometheus
/var/log/prometheus
