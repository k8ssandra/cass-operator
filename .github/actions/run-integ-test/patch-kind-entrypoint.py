"""Keep Docker's loopback DNS when Kind has no IPv4 host address.

Temporary workaround for kubernetes-sigs/kind#4152. Preserve the later
IPv6 address/certificate fixups in enable_network_magic(). CI configures
CoreDNS separately with an IPv6-reachable DNS forwarder.
"""

import pathlib
import sys

path = pathlib.Path(sys.argv[1])
source = path.read_text()
start = "  # patch docker's iptables rules to switch out the DNS IP\n"
end = "  local files_to_update=(\n"
if source.count(start) != 1 or source.count(end) != 1:
    raise SystemExit("Kind entrypoint changed; review the IPv6 DNS workaround")
before, remainder = source.split(start)
dns, after = remainder.split(end)
path.write_text(
    before
    + '  if [[ -n "${docker_host_ip}" ]]; then\n'
    + start
    + dns
    + "  else\n"
    + "    log_info 'keeping Docker loopback DNS on IPv6-only node'\n"
    + "  fi\n"
    + end
    + after
)
