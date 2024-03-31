"""
   Very basic mDNS scanner with HTML table renderer
"""
import subprocess

class MdnsHosts:
    """mDNS scanner class using avahi-browse"""
    def hask(self):
        """Check if avahi-browse has -k option"""
        try:
            result = subprocess.run(['avahi-browse', '--help'], check=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            return '-k' in result.stdout
        except subprocess.CalledProcessError:
            return False

    def scan(self):
        """Perform mDNS scan and return list of hosts."""
        services = {
            '_http._tcp':        ('HTTP', 'http://{address}:{port}{path}'),
            '_https._tcp':       ('HTTPS', 'https://{address}:{port}{path}'),
            '_netconf-ssh._tcp': ('NETCONF', None),
            '_ssh._tcp':         ('SSH', None),
            '_sftp-ssh._tcp':    ('SFTP', None),
        }

        result = subprocess.run(['avahi-browse', '-tarpk' if self.hask() else '-tarp'],
                                stdout=subprocess.PIPE, text=True)
        lines = result.stdout.strip().split('\n')
        hosts_services = {}

        for line in lines:
            print(f"{line}")

            parts = line.split(';')
            if len(parts) <= 8 or parts[0] != '=':
                continue

            family = parts[2]
            service_name = parts[3]
            service_type = parts[4]
            link = parts[6]
            address = parts[7]
            port = parts[8]
            txt = parts[9]

            if family not in ('IPv4', 'IPv6'):
                continue

            if service_type in services:
                identifier, url_template = services[service_type]
                other = False
            else:
                identifier = service_type
                url_template = None
                other = True

            path = ""
            records = txt.split(' ')
            for record in records:
                stripped = record.strip("\"")
                if "path=" in stripped:
                    path = stripped.split('path=')[-1]
                    break

            if url_template:
                url = url_template.format(address=address, port=port, path=path)
            else:
                url = None

            service_details = {
                'type': identifier,
                'name': self.decode(service_name),
                'url': url,
                'other': other
            }

            if link not in hosts_services:
                hosts_services[link] = {'services': [service_details]}
            elif service_details not in hosts_services[link]['services']:
                hosts_services[link]['services'].append(service_details)

        return hosts_services

    def decode(self, name):
        """Decode escape sequences like \032 and \040 in service names"""
        name = name.replace('\\032', ' ')
        name = name.replace('\\040', '(')
        name = name.replace('\\041', ')')
        return bytes(name, "utf-8").decode("unicode_escape")
