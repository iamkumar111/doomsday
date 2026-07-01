#!/usr/bin/env python3
# GODINTEL v12 — ENTERPRISE-GRADE CDN BYPASS & RECON ENGINE
# Deep Domain Intelligence + Real IP Discovery + Technology Fingerprinting
# Inspired by CloakQuest3r with advanced enhancements
# Author: Dimpal Sharma | DevSecOps Engineer | Cloud Security Specialist

import requests, re, sys, json, time, socket, ssl, os, threading, urllib.request, configparser
from datetime import datetime
from urllib.parse import urlparse
from concurrent.futures import ThreadPoolExecutor, as_completed

import warnings
warnings.filterwarnings('ignore')

# Optional imports for SSL certificate analysis
try:
    from cryptography import x509
    from cryptography.hazmat.backends import default_backend
    CRYPTO_AVAILABLE = True
except ImportError:
    CRYPTO_AVAILABLE = False

# Optional import for HTML parsing
try:
    from bs4 import BeautifulSoup
    BS4_AVAILABLE = True
except ImportError:
    BS4_AVAILABLE = False

VERSION = "12.0.0"
CONFIG_FILE = "config.ini"

class Colors:
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    END = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'

# ═══════════════════════════════════════════════════════════════════════════════
# CONFIGURATION MANAGEMENT
# ═══════════════════════════════════════════════════════════════════════════════

def read_config():
    """Read API keys from config file"""
    config = configparser.ConfigParser()
    if not os.path.exists(CONFIG_FILE):
        config["DEFAULT"] = {
            "securitytrails_api_key": "your_api_key_here",
            "shodan_api_key": "your_shodan_key_here"
        }
        with open(CONFIG_FILE, 'w') as f:
            config.write(f)
        return None
    else:
        config.read(CONFIG_FILE)
        api_key = config['DEFAULT'].get('securitytrails_api_key', '')
        if api_key and api_key != 'your_api_key_here':
            return api_key
        return None

def get_shodan_key():
    """Get Shodan API key from config"""
    config = configparser.ConfigParser()
    if os.path.exists(CONFIG_FILE):
        config.read(CONFIG_FILE)
        key = config['DEFAULT'].get('shodan_api_key', '')
        if key and key != 'your_shodan_key_here':
            return key
    return None

# ═══════════════════════════════════════════════════════════════════════════════
# BANNER & UI
# ═══════════════════════════════════════════════════════════════════════════════

def banner():
    print(f"""{Colors.CYAN}
    ╔═══════════════════════════════════════════════════════════════════════╗
    ║       ██████╗  ██████╗ ██████╗ ██╗███╗   ██╗████████╗███████╗██╗      ║
    ║      ██╔════╝ ██╔═══██╗██╔══██╗██║████╗  ██║╚══██╔══╝██╔════╝██║      ║
    ║      ██║  ███╗██║   ██║██║  ██║██║██╔██╗ ██║   ██║   █████╗  ██║      ║
    ║      ██║   ██║██║   ██║██║  ██║██║██║╚██╗██║   ██║   ██╔══╝  ██║      ║
    ║      ╚██████╔╝╚██████╔╝██████╔╝██║██║ ╚████║   ██║   ███████╗███████╗ ║
    ║       ╚═════╝  ╚═════╝ ╚═════╝ ╚═╝╚═╝  ╚═══╝   ╚═╝   ╚══════╝╚══════╝ ║
    ║                                                                       ║
    ║         ENTERPRISE CDN BYPASS & ORIGIN IP DISCOVERY ENGINE            ║
    ╚═══════════════════════════════════════════════════════════════════════╝{Colors.END}
    """)
    print(f"{Colors.GREEN}[+] {Colors.YELLOW}Version      : {Colors.END}{VERSION}")
    print(f"{Colors.GREEN}[+] {Colors.YELLOW}Author       : {Colors.END}Dimpal Sharma")
    print(f"{Colors.GREEN}[+] {Colors.YELLOW}Role         : {Colors.END}DevSecOps Engineer | Cloud Security")
    print(f"{Colors.GREEN}[+] {Colors.YELLOW}Contact      : {Colors.END}sharamhu16@gmail.com")
    print(f"{Colors.GREEN}[+] {Colors.YELLOW}Scan Time    : {Colors.END}{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")

# ═══════════════════════════════════════════════════════════════════════════════
# CLOUDFLARE DETECTION
# ═══════════════════════════════════════════════════════════════════════════════

def is_using_cloudflare(domain):
    """Detect if target is protected by Cloudflare"""
    try:
        response = requests.head(f"https://{domain}", timeout=10, verify=False)
        headers = response.headers
        
        # Check various Cloudflare indicators
        if "server" in headers and "cloudflare" in headers["server"].lower():
            return True
        if "cf-ray" in headers:
            return True
        if any("cloudflare" in str(v).lower() for v in headers.values()):
            return True
            
        # Check for Cloudflare IP ranges
        try:
            ip = socket.gethostbyname(domain)
            cf_ranges = ["104.", "103.21", "103.22", "103.31", "141.", "162.", "172.", 
                        "173.", "188.114", "190.", "197.234", "198.41"]
            for cf_range in cf_ranges:
                if ip.startswith(cf_range):
                    return True
        except:
            pass
            
    except:
        pass
    return False

def detect_cdn_provider(domain):
    """Detect which CDN/WAF is protecting the domain"""
    try:
        response = requests.head(f"https://{domain}", timeout=10, verify=False)
        headers = response.headers
        
        providers = []
        
        # Cloudflare
        if "cf-ray" in headers or ("server" in headers and "cloudflare" in headers["server"].lower()):
            providers.append("Cloudflare")
        
        # Akamai
        if any(h in headers for h in ["X-Akamai-Transformed", "Akamai-Origin-Hop"]):
            providers.append("Akamai")
        
        # Fastly
        if "X-Fastly-Request-ID" in headers or "Fastly-Debug-Digest" in headers:
            providers.append("Fastly")
        
        # AWS CloudFront
        if "X-Amz-Cf-Id" in headers or "X-Amz-Cf-Pop" in headers:
            providers.append("AWS CloudFront")
        
        # Sucuri
        if "X-Sucuri-ID" in headers:
            providers.append("Sucuri WAF")
        
        # Imperva/Incapsula
        if "X-CDN" in headers and "imperva" in headers["X-CDN"].lower():
            providers.append("Imperva/Incapsula")
        
        # Google Cloud CDN
        if "X-GUploader-UploadID" in headers:
            providers.append("Google Cloud CDN")
        
        return providers if providers else ["Unknown/Direct"]
        
    except:
        return ["Detection Failed"]

# ═══════════════════════════════════════════════════════════════════════════════
# SSL CERTIFICATE ANALYSIS
# ═══════════════════════════════════════════════════════════════════════════════

def get_ssl_certificate_info(host):
    """Extract SSL certificate information for origin verification"""
    if not CRYPTO_AVAILABLE:
        return None
    
    try:
        context = ssl.create_default_context()
        with context.wrap_socket(socket.socket(), server_hostname=host) as sock:
            sock.settimeout(10)
            sock.connect((host, 443))
            certificate_der = sock.getpeercert(True)

        certificate = x509.load_der_x509_certificate(certificate_der, default_backend())
        
        common_name = certificate.subject.get_attributes_for_oid(x509.NameOID.COMMON_NAME)
        cn = common_name[0].value if common_name else "N/A"
        
        issuer_cn = certificate.issuer.get_attributes_for_oid(x509.NameOID.COMMON_NAME)
        issuer = issuer_cn[0].value if issuer_cn else "N/A"
        
        # Get Subject Alternative Names
        try:
            san_extension = certificate.extensions.get_extension_for_oid(x509.ExtensionOID.SUBJECT_ALTERNATIVE_NAME)
            sans = [name.value for name in san_extension.value]
        except:
            sans = []

        return {
            "Common Name": cn,
            "Issuer": issuer,
            "Valid From": str(certificate.not_valid_before_utc),
            "Valid Until": str(certificate.not_valid_after_utc),
            "SANs": sans[:5]  # Limit to 5
        }
    except Exception as e:
        return None

# ═══════════════════════════════════════════════════════════════════════════════
# HISTORICAL IP LOOKUP
# ═══════════════════════════════════════════════════════════════════════════════

def get_viewdns_history(domain):
    """Get historical IP addresses from ViewDNS.info"""
    if not BS4_AVAILABLE:
        print(f"{Colors.YELLOW}    [!] BeautifulSoup not installed. Skipping ViewDNS lookup.{Colors.END}")
        return []
    
    try:
        url = f"https://viewdns.info/iphistory/?domain={domain}"
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
        }
        response = requests.get(url, headers=headers, timeout=15)
        soup = BeautifulSoup(response.text, 'html.parser')
        table = soup.find('table', {'border': '1'})
        
        historical_ips = []
        if table:
            rows = table.find_all('tr')[2:]  # Skip header rows
            for row in rows:
                cols = row.find_all('td')
                if len(cols) >= 4:
                    historical_ips.append({
                        "ip": cols[0].text.strip(),
                        "location": cols[1].text.strip(),
                        "owner": cols[2].text.strip(),
                        "last_seen": cols[3].text.strip()
                    })
        return historical_ips
    except:
        return []

def get_securitytrails_history(domain):
    """Get historical IP data from SecurityTrails API"""
    api_key = read_config()
    if not api_key:
        return []
    
    try:
        url = f"https://api.securitytrails.com/v1/history/{domain}/dns/a"
        headers = {
            "accept": "application/json",
            "APIKEY": api_key
        }
        response = requests.get(url, headers=headers, timeout=15)
        data = response.json()
        
        historical_ips = []
        for record in data.get('records', []):
            for value in record.get('values', []):
                historical_ips.append({
                    "ip": value.get('ip', 'N/A'),
                    "first_seen": record.get('first_seen', 'N/A'),
                    "last_seen": record.get('last_seen', 'N/A'),
                    "organization": record.get('organizations', ['Unknown'])[0] if record.get('organizations') else 'Unknown'
                })
        return historical_ips
    except:
        return []

# ═══════════════════════════════════════════════════════════════════════════════
# SUBDOMAIN ENUMERATION WITH THREADING
# ═══════════════════════════════════════════════════════════════════════════════

WORDLIST_URL = "https://github.com/danielmiessler/SecLists/raw/master/Discovery/DNS/subdomains-top1million-5000.txt"
DEFAULT_WORDLIST = "wordlist.txt"

def download_wordlist():
    """Download subdomain wordlist from SecLists"""
    if os.path.exists(DEFAULT_WORDLIST):
        return DEFAULT_WORDLIST
    
    print(f"{Colors.BLUE}[+] Downloading subdomain wordlist from SecLists...{Colors.END}")
    try:
        urllib.request.urlretrieve(WORDLIST_URL, DEFAULT_WORDLIST)
        print(f"{Colors.GREEN}[✓] Wordlist downloaded: {DEFAULT_WORDLIST}{Colors.END}")
        return DEFAULT_WORDLIST
    except Exception as e:
        print(f"{Colors.RED}[!] Failed to download wordlist: {e}{Colors.END}")
        return None

def check_subdomain(subdomain, domain, timeout=10):
    """Check if a subdomain exists and return its IP"""
    full_domain = f"{subdomain}.{domain}"
    try:
        ip = socket.gethostbyname(full_domain)
        # Verify with HTTP request
        try:
            requests.get(f"https://{full_domain}", timeout=timeout, verify=False)
        except:
            try:
                requests.get(f"http://{full_domain}", timeout=timeout)
            except:
                pass
        return (full_domain, ip)
    except:
        return None

def subdomain_scan(domain, wordlist_path=None, max_threads=50):
    """Threaded subdomain scanning with SSL analysis"""
    print(f"\n{Colors.YELLOW}[★] Phase 6 — Subdomain Enumeration & SSL Analysis{Colors.END}")
    
    if wordlist_path is None:
        wordlist_path = download_wordlist()
        if not wordlist_path:
            print(f"{Colors.RED}[!] No wordlist available. Skipping subdomain scan.{Colors.END}")
            return []
    
    try:
        with open(wordlist_path, 'r') as f:
            subdomains = [line.strip() for line in f if line.strip()]
    except:
        print(f"{Colors.RED}[!] Failed to read wordlist.{Colors.END}")
        return []
    
    print(f"{Colors.CYAN}[→] Scanning {len(subdomains)} potential subdomains with {max_threads} threads...{Colors.END}")
    
    found_subdomains = []
    start_time = time.time()
    
    with ThreadPoolExecutor(max_workers=max_threads) as executor:
        futures = {executor.submit(check_subdomain, sub, domain): sub for sub in subdomains}
        
        for future in as_completed(futures):
            result = future.result()
            if result:
                found_subdomains.append(result)
                print(f"{Colors.GREEN}    [✓] Found: {result[0]} → {result[1]}{Colors.END}")
    
    elapsed = time.time() - start_time
    
    print(f"\n{Colors.GREEN}[✓] Subdomain Scan Complete:{Colors.END}")
    print(f"    ├─ Subdomains Scanned: {len(subdomains)}")
    print(f"    ├─ Subdomains Found: {len(found_subdomains)}")
    print(f"    └─ Time Elapsed: {elapsed:.2f} seconds")
    
    # Analyze SSL certificates for found subdomains
    if found_subdomains and CRYPTO_AVAILABLE:
        print(f"\n{Colors.YELLOW}[★] SSL Certificate Analysis for Subdomains:{Colors.END}")
        for subdomain, ip in found_subdomains[:10]:  # Limit to 10
            ssl_info = get_ssl_certificate_info(subdomain)
            if ssl_info:
                print(f"\n{Colors.CYAN}    [{subdomain}] → {ip}{Colors.END}")
                for key, value in ssl_info.items():
                    if key != "SANs":
                        print(f"      ├─ {key}: {value}")
                    elif value:
                        print(f"      └─ SANs: {', '.join(value[:3])}")
    
    return found_subdomains

# ═══════════════════════════════════════════════════════════════════════════════
# IP DISCOVERY (Original Enhanced)
# ═══════════════════════════════════════════════════════════════════════════════

def get_real_ip(domain):
    """Multi-layer IP discovery bypassing CDN/Firewall"""
    print(f"\n{Colors.YELLOW}[★] Phase 1 — Real IP Discovery (CDN Bypass){Colors.END}")
    print(f"{Colors.CYAN}[→] Target: {domain}{Colors.END}")
    
    ips = []
    cdn_ranges = [
        "104.", "141.", "162.", "172.", "173.", "190.", "198.41", "185.",
        "103.21", "103.22", "103.31", "131.0.72", "188.114", "197.234", "198.41"
    ]
    
    # Method 1: DNS Lookup
    try:
        print(f"{Colors.BLUE}[+] DNS Lookup...{Colors.END}")
        ip = socket.gethostbyname(domain)
        ips.append(ip)
        print(f"    ├─ Primary DNS: {ip}")
    except:
        pass
    
    # Method 2: HackerTarget API
    try:
        print(f"{Colors.BLUE}[+] Querying HackerTarget...{Colors.END}")
        r = requests.get(f"https://api.hackertarget.com/dnslookup/?q={domain}", timeout=10)
        found = re.findall(r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', r.text)
        for ip in found:
            if ip not in ips:
                ips.append(ip)
                print(f"    ├─ Found: {ip}")
    except:
        pass
    
    # Method 3: Certificate Transparency Logs
    try:
        print(f"{Colors.BLUE}[+] Certificate Transparency Search...{Colors.END}")
        r = requests.get(f"https://crt.sh/?q=%25.{domain}&output=json", timeout=15)
        if r.status_code == 200:
            data = json.loads(r.text)
            subdomains_checked = set()
            for entry in data[:20]:
                name = entry.get('name_value', '')
                for subdomain in name.split('\n'):
                    if domain in subdomain and '*' not in subdomain and subdomain not in subdomains_checked:
                        subdomains_checked.add(subdomain)
                        try:
                            ip = socket.gethostbyname(subdomain)
                            if ip not in ips:
                                ips.append(ip)
                                print(f"    ├─ CT Log IP: {ip} ({subdomain})")
                        except:
                            pass
    except:
        pass
    
    # Method 4: DNS BufferOver
    try:
        print(f"{Colors.BLUE}[+] DNS BufferOver Scan...{Colors.END}")
        r = requests.get(f"https://dns.bufferover.run/dns?q={domain}", timeout=12)
        found = re.findall(r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', r.text)
        for ip in found[:10]:
            if ip not in ips:
                ips.append(ip)
    except:
        pass
    
    # Method 5: Shodan (if API key available)
    shodan_key = get_shodan_key()
    if shodan_key:
        try:
            print(f"{Colors.BLUE}[+] Shodan Search...{Colors.END}")
            r = requests.get(f"https://api.shodan.io/dns/resolve?hostnames={domain}&key={shodan_key}", timeout=10)
            data = r.json()
            if domain in data:
                ip = data[domain]
                if ip and ip not in ips:
                    ips.append(ip)
                    print(f"    ├─ Shodan: {ip}")
        except:
            pass
    
    # Method 6: MX Records (mail servers often reveal origin)
    try:
        print(f"{Colors.BLUE}[+] MX Record Analysis...{Colors.END}")
        r = requests.get(f"https://api.hackertarget.com/mtr/?q={domain}", timeout=10)
        found = re.findall(r'\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b', r.text)
        for ip in found[:5]:
            if ip not in ips:
                ips.append(ip)
                print(f"    ├─ MX IP: {ip}")
    except:
        pass
    
    # Filter out CDN IPs
    real_ips = []
    cdn_ips = []
    
    for ip in ips:
        is_cdn = False
        for cdn in cdn_ranges:
            if ip.startswith(cdn):
                cdn_ips.append(ip)
                is_cdn = True
                break
        if not is_cdn:
            real_ips.append(ip)
    
    print(f"\n{Colors.GREEN}[✓] IP Discovery Complete:{Colors.END}")
    if real_ips:
        print(f"    ├─ Potential Origin IPs: {', '.join(real_ips[:5])}")
    if cdn_ips:
        print(f"    └─ CDN/Proxy IPs: {', '.join(cdn_ips[:5])}")
    
    return real_ips[0] if real_ips else (ips[0] if ips else None), real_ips, cdn_ips

# ═══════════════════════════════════════════════════════════════════════════════
# IP INTELLIGENCE
# ═══════════════════════════════════════════════════════════════════════════════

def get_ip_info(ip):
    """Deep IP intelligence gathering"""
    print(f"\n{Colors.YELLOW}[★] Phase 2 — IP Intelligence Analysis{Colors.END}")
    
    info = {}
    
    try:
        r = requests.get(f"https://ipwho.is/{ip}", timeout=10).json()
        info['country'] = r.get('country', 'Unknown')
        info['country_code'] = r.get('country_code', 'XX')
        info['city'] = r.get('city', 'Unknown')
        info['region'] = r.get('region', 'Unknown')
        info['isp'] = r.get('connection', {}).get('isp', 'Unknown')
        info['org'] = r.get('connection', {}).get('org', 'Unknown')
        info['asn'] = r.get('connection', {}).get('asn', 'Unknown')
        info['timezone'] = r.get('timezone', {}).get('id', 'Unknown')
        info['latitude'] = r.get('latitude', 'Unknown')
        info['longitude'] = r.get('longitude', 'Unknown')
        
        print(f"{Colors.GREEN}[✓] Geolocation Data Retrieved{Colors.END}")
    except:
        print(f"{Colors.RED}[!] Failed to get IP info{Colors.END}")
        info = {'country': 'Unknown', 'country_code': 'XX', 'city': 'Unknown', 'isp': 'Unknown', 
                'org': 'Unknown', 'asn': 'Unknown', 'timezone': 'Unknown', 'region': 'Unknown',
                'latitude': 'Unknown', 'longitude': 'Unknown'}
    
    # Identify hosting provider
    org = info['org'].lower()
    providers = {
        "hetzner": "Hetzner Online GmbH (Germany)",
        "ovh": "OVH SAS (France)",
        "hostinger": "Hostinger International",
        "digitalocean": "DigitalOcean LLC",
        "amazon": "Amazon Web Services (AWS)",
        "aws": "Amazon Web Services (AWS)",
        "google": "Google Cloud Platform",
        "gcp": "Google Cloud Platform",
        "cloudflare": "Cloudflare Inc. (CDN)",
        "contabo": "Contabo GmbH",
        "namecheap": "Namecheap Inc.",
        "godaddy": "GoDaddy",
        "linode": "Linode (Akamai)",
        "vultr": "Vultr Holdings",
        "azure": "Microsoft Azure",
        "microsoft": "Microsoft Azure",
        "alibaba": "Alibaba Cloud",
        "oracle": "Oracle Cloud"
    }
    
    info['provider'] = info['org']
    for key, value in providers.items():
        if key in org:
            info['provider'] = value
            break
    
    return info

# ═══════════════════════════════════════════════════════════════════════════════
# TECHNOLOGY FINGERPRINTING
# ═══════════════════════════════════════════════════════════════════════════════

def check_headers_direct(ip, domain, verbose=False):
    """Check headers by directly connecting to IP (silent on failure)"""
    try:
        headers = {
            'Host': domain,
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }
        
        try:
            r = requests.get(f"https://{ip}", headers=headers, timeout=5, verify=False)
            if verbose:
                print(f"{Colors.GREEN}[✓] Direct IP bypass successful (HTTPS){Colors.END}")
            return r.headers, r.text
        except:
            r = requests.get(f"http://{ip}", headers=headers, timeout=5)
            if verbose:
                print(f"{Colors.GREEN}[✓] Direct IP bypass successful (HTTP){Colors.END}")
            return r.headers, r.text
    except:
        # Silent failure - direct IP connection is often blocked
        return None, None

def analyze_technology(domain, headers, html, ip=None):
    """Advanced technology fingerprinting"""
    print(f"\n{Colors.YELLOW}[★] Phase 3 — Technology Stack Fingerprinting{Colors.END}")
    
    tech_stack = {
        'server': 'Unknown',
        'language': [],
        'framework': [],
        'cms': [],
        'cdn': [],
        'analytics': [],
        'javascript': [],
        'other': []
    }
    
    if ip:
        direct_headers, direct_html = check_headers_direct(ip, domain)
        if direct_headers:
            headers = direct_headers
            html = direct_html
            print(f"{Colors.CYAN}[→] Analyzing direct server response...{Colors.END}")
    
    if not html:
        try:
            r = requests.get(f"https://{domain}", timeout=10, verify=False)
            headers = r.headers
            html = r.text
        except:
            try:
                r = requests.get(f"http://{domain}", timeout=10)
                headers = r.headers
                html = r.text
            except:
                pass
    
    # Server Detection
    server = headers.get('Server', headers.get('server', 'Unknown'))
    tech_stack['server'] = server
    
    print(f"{Colors.BLUE}[+] Server Analysis:{Colors.END}")
    print(f"    ├─ Server Header: {server}")
    
    # Language Detection
    x_powered = headers.get('X-Powered-By', headers.get('x-powered-by', ''))
    if x_powered:
        tech_stack['language'].append(x_powered)
        print(f"    ├─ X-Powered-By: {x_powered}")
    
    if 'PHP' in x_powered.upper() or 'PHPSESSID' in str(headers):
        tech_stack['language'].append('PHP')
    
    if any(h in headers for h in ['X-AspNet-Version', 'X-AspNetMvc-Version']):
        tech_stack['language'].append('ASP.NET')
    
    if any(s in server.lower() for s in ['waitress', 'gunicorn', 'uwsgi']):
        tech_stack['language'].append('Python')
    
    if 'express' in server.lower() or ('X-Powered-By' in headers and 'Express' in headers['X-Powered-By']):
        tech_stack['language'].append('Node.js')
        tech_stack['framework'].append('Express.js')
    
    # CDN Detection
    cdn_headers = ['CF-Ray', 'X-CDN', 'X-Akamai', 'X-Cache', 'X-Fastly-Request-ID']
    for cdn_h in cdn_headers:
        if cdn_h in headers:
            if 'CF-' in cdn_h:
                tech_stack['cdn'].append('Cloudflare')
            elif 'Akamai' in cdn_h:
                tech_stack['cdn'].append('Akamai')
            elif 'Fastly' in cdn_h:
                tech_stack['cdn'].append('Fastly')
    
    # HTML Analysis
    if html:
        html_lower = html.lower()
        
        print(f"{Colors.BLUE}[+] HTML Content Analysis:{Colors.END}")
        
        # CMS Detection
        cms_patterns = {
            'WordPress': ['wp-content', 'wp-includes', '/wp-json/', 'wordpress'],
            'Joomla': ['joomla', '/components/com_'],
            'Drupal': ['drupal', '/sites/default/'],
            'Magento': ['magento', 'mage/cookies'],
            'Shopify': ['shopify', 'cdn.shopify.com'],
            'Wix': ['wix.com', 'wixstatic'],
            'Squarespace': ['squarespace'],
            'Ghost': ['ghost'],
            'Webflow': ['webflow'],
            'HubSpot': ['hubspot'],
        }
        
        for cms, patterns in cms_patterns.items():
            if any(p in html_lower for p in patterns):
                tech_stack['cms'].append(cms)
                print(f"    ├─ CMS Detected: {cms}")
        
        # JavaScript Framework Detection
        js_frameworks = {
            'React': ['react', '_react', 'reactjs'],
            'Vue.js': ['vue.js', 'data-v-', '__vue__'],
            'Angular': ['ng-version', 'angular', 'ng-app'],
            'Next.js': ['__next', '_next/'],
            'Nuxt.js': ['__nuxt', '_nuxt/'],
            'Svelte': ['svelte'],
            'jQuery': ['jquery'],
            'Alpine.js': ['x-data', 'alpine'],
            'HTMX': ['htmx.org', 'hx-'],
        }
        
        for fw, patterns in js_frameworks.items():
            if any(p in html_lower for p in patterns):
                tech_stack['javascript'].append(fw)
                print(f"    ├─ JS Framework: {fw}")
        
        # Analytics Detection
        analytics = {
            'Google Analytics': ['google-analytics', 'gtag', 'ga.js'],
            'Google Tag Manager': ['googletagmanager'],
            'Facebook Pixel': ['facebook.com/tr', 'fbq('],
            'Hotjar': ['hotjar'],
            'Mixpanel': ['mixpanel'],
            'Segment': ['segment.com', 'analytics.js'],
            'Plausible': ['plausible.io'],
        }
        
        for tool, patterns in analytics.items():
            if any(p in html_lower for p in patterns):
                tech_stack['analytics'].append(tool)
        
        # Additional Technologies
        if 'bootstrap' in html_lower:
            tech_stack['other'].append('Bootstrap')
        if 'tailwind' in html_lower:
            tech_stack['other'].append('Tailwind CSS')
        if 'fontawesome' in html_lower or 'font-awesome' in html_lower:
            tech_stack['other'].append('Font Awesome')
    
    return tech_stack

# ═══════════════════════════════════════════════════════════════════════════════
# WHOIS & SECURITY HEADERS
# ═══════════════════════════════════════════════════════════════════════════════

def whois_lookup(domain):
    """WHOIS information gathering"""
    print(f"\n{Colors.YELLOW}[★] Phase 4 — Domain Registration Intelligence{Colors.END}")
    
    try:
        r = requests.get(f"https://www.whoisxmlapi.com/whoisserver/WhoisService?apiKey=at_free&domainName={domain}&outputFormat=JSON", timeout=10)
        data = r.json()
        
        whois_data = data.get('WhoisRecord', {})
        
        info = {
            'registrar': whois_data.get('registrarName', 'Unknown'),
            'creation_date': whois_data.get('createdDate', 'Unknown'),
            'expiry_date': whois_data.get('expiresDate', 'Unknown'),
            'updated_date': whois_data.get('updatedDate', 'Unknown'),
            'status': whois_data.get('status', 'Unknown'),
            'nameservers': whois_data.get('nameServers', {}).get('hostNames', [])
        }
        
        print(f"{Colors.GREEN}[✓] WHOIS Data Retrieved{Colors.END}")
        return info
    except:
        print(f"{Colors.RED}[!] WHOIS lookup unavailable{Colors.END}")
        return None

def security_headers_check(headers):
    """Security headers analysis"""
    print(f"\n{Colors.YELLOW}[★] Phase 5 — Security Headers Analysis{Colors.END}")
    
    security_headers = {
        'Strict-Transport-Security': 'HSTS',
        'Content-Security-Policy': 'CSP',
        'X-Frame-Options': 'Clickjacking Protection',
        'X-Content-Type-Options': 'MIME Sniffing Protection',
        'X-XSS-Protection': 'XSS Protection',
        'Referrer-Policy': 'Referrer Policy',
        'Permissions-Policy': 'Permissions Policy'
    }
    
    results = {}
    for header, name in security_headers.items():
        if header in headers:
            results[name] = f"✓ Enabled"
            print(f"{Colors.GREEN}    [✓] {name}: Enabled{Colors.END}")
        else:
            results[name] = "✗ Missing"
            print(f"{Colors.RED}    [✗] {name}: Missing{Colors.END}")
    
    return results

# ═══════════════════════════════════════════════════════════════════════════════
# REPORT GENERATION
# ═══════════════════════════════════════════════════════════════════════════════

def generate_report(domain, primary_ip, all_real_ips, cdn_ips, ip_info, tech_stack, 
                   whois_data, security_headers, historical_ips, subdomains_found, cdn_providers):
    """Generate comprehensive intelligence report"""
    
    print(f"\n\n{Colors.BOLD}{Colors.CYAN}{'═'*80}{Colors.END}")
    print(f"{Colors.BOLD}{Colors.CYAN}                    GODINTEL v12 — INTELLIGENCE REPORT{Colors.END}")
    print(f"{Colors.BOLD}{Colors.CYAN}{'═'*80}{Colors.END}\n")
    
    # Target Information
    print(f"{Colors.BOLD}[TARGET DOMAIN]{Colors.END}")
    print(f"  Domain               : {domain}")
    print(f"  Scan Time            : {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"  CDN/WAF Detected     : {', '.join(cdn_providers)}")
    
    # IP Information
    print(f"\n{Colors.BOLD}[NETWORK INFRASTRUCTURE]{Colors.END}")
    print(f"  Primary IP           : {primary_ip}")
    if all_real_ips and len(all_real_ips) > 1:
        print(f"  Additional IPs       : {', '.join(all_real_ips[1:5])}")
    if cdn_ips:
        print(f"  CDN/Proxy IPs        : {', '.join(cdn_ips[:5])}")
    
    # Historical IPs
    if historical_ips:
        print(f"\n{Colors.BOLD}[HISTORICAL IP DATA]{Colors.END}")
        for i, hist in enumerate(historical_ips[:5]):
            if 'last_seen' in hist:
                print(f"  [{i+1}] {hist.get('ip', 'N/A')} - Last seen: {hist.get('last_seen', 'N/A')}")
    
    # Hosting Information
    print(f"\n{Colors.BOLD}[HOSTING PROVIDER]{Colors.END}")
    print(f"  Provider             : {ip_info.get('provider', 'Unknown')}")
    print(f"  Organization         : {ip_info.get('org', 'Unknown')}")
    print(f"  ASN                  : {ip_info.get('asn', 'Unknown')}")
    print(f"  ISP                  : {ip_info.get('isp', 'Unknown')}")
    
    # Geolocation
    print(f"\n{Colors.BOLD}[GEOLOCATION]{Colors.END}")
    print(f"  Country              : {ip_info.get('country', 'Unknown')} ({ip_info.get('country_code', 'XX')})")
    print(f"  Region               : {ip_info.get('region', 'Unknown')}")
    print(f"  City                 : {ip_info.get('city', 'Unknown')}")
    print(f"  Timezone             : {ip_info.get('timezone', 'Unknown')}")
    
    # Subdomains
    if subdomains_found:
        print(f"\n{Colors.BOLD}[SUBDOMAINS DISCOVERED]{Colors.END}")
        print(f"  Total Found          : {len(subdomains_found)}")
        for subdomain, ip in subdomains_found[:10]:
            print(f"    ├─ {subdomain} → {ip}")
    
    # Technology Stack
    print(f"\n{Colors.BOLD}[TECHNOLOGY STACK]{Colors.END}")
    print(f"  Web Server           : {tech_stack['server']}")
    
    if tech_stack['language']:
        print(f"  Backend Language     : {', '.join(set(tech_stack['language']))}")
    
    if tech_stack['cms']:
        print(f"  CMS Platform         : {', '.join(set(tech_stack['cms']))}")
    
    if tech_stack['framework']:
        print(f"  Framework            : {', '.join(set(tech_stack['framework']))}")
    
    if tech_stack['javascript']:
        print(f"  JavaScript Libs      : {', '.join(set(tech_stack['javascript']))}")
    
    if tech_stack['cdn']:
        print(f"  CDN Services         : {', '.join(set(tech_stack['cdn']))}")
    
    if tech_stack['analytics']:
        print(f"  Analytics Tools      : {', '.join(set(tech_stack['analytics']))}")
    
    # WHOIS Information
    if whois_data:
        print(f"\n{Colors.BOLD}[DOMAIN REGISTRATION]{Colors.END}")
        print(f"  Registrar            : {whois_data.get('registrar', 'Unknown')}")
        print(f"  Created Date         : {str(whois_data.get('creation_date', 'Unknown'))[:10]}")
        print(f"  Expiry Date          : {str(whois_data.get('expiry_date', 'Unknown'))[:10]}")
        if whois_data.get('nameservers'):
            print(f"  Name Servers         : {', '.join(whois_data['nameservers'][:3])}")
    
    # Security Headers
    print(f"\n{Colors.BOLD}[SECURITY POSTURE]{Colors.END}")
    enabled = sum(1 for v in security_headers.values() if '✓' in v)
    total = len(security_headers)
    score = (enabled / total) * 100
    print(f"  Security Score       : {score:.0f}% ({enabled}/{total} headers)")
    
    print(f"\n{Colors.BOLD}{Colors.CYAN}{'═'*80}{Colors.END}")
    print(f"{Colors.GREEN}[✓] GODINTEL v12 Reconnaissance Complete{Colors.END}\n")

# ═══════════════════════════════════════════════════════════════════════════════
# MAIN EXECUTION
# ═══════════════════════════════════════════════════════════════════════════════

def main():
    banner()
    
    if len(sys.argv) != 2:
        print(f"{Colors.RED}Usage: python3 godintelv11.py <domain>{Colors.END}")
        print(f"{Colors.YELLOW}Example: python3 godintelv11.py example.com{Colors.END}")
        sys.exit(1)
    
    domain = sys.argv[1].replace('http://', '').replace('https://', '').replace('www.', '').strip('/')
    
    # Detect CDN/WAF
    print(f"\n{Colors.CYAN}[!] Checking CDN/WAF Protection...{Colors.END}")
    cdn_providers = detect_cdn_provider(domain)
    is_cloudflare = is_using_cloudflare(domain)
    
    if is_cloudflare:
        print(f"{Colors.RED}[!] Target is protected by Cloudflare{Colors.END}")
    else:
        print(f"{Colors.GREEN}[✓] CDN Detection: {', '.join(cdn_providers)}{Colors.END}")
    
    # Phase 1: IP Discovery
    primary_ip, real_ips, cdn_ips = get_real_ip(domain)
    
    if not primary_ip:
        print(f"{Colors.RED}[!] Failed to resolve domain{Colors.END}")
        sys.exit(1)
    
    # Phase 2: IP Intelligence
    ip_info = get_ip_info(primary_ip)
    
    # Phase 3: Technology Detection
    headers = {}
    html = ""
    
    try:
        r = requests.get(f"https://{domain}", timeout=10, verify=False)
        headers = r.headers
        html = r.text
    except:
        try:
            r = requests.get(f"http://{domain}", timeout=10)
            headers = r.headers
            html = r.text
        except:
            print(f"{Colors.RED}[!] Unable to fetch domain content{Colors.END}")
    
    check_ip = real_ips[0] if real_ips else None
    tech_stack = analyze_technology(domain, headers, html, check_ip)
    
    # Phase 4: WHOIS Lookup
    whois_data = whois_lookup(domain)
    
    # Phase 5: Security Headers
    security_headers_result = security_headers_check(headers)
    
    # Phase 6: Historical IP Lookup
    print(f"\n{Colors.YELLOW}[★] Historical IP Address Analysis{Colors.END}")
    historical_ips = []
    
    viewdns_history = get_viewdns_history(domain)
    if viewdns_history:
        print(f"{Colors.GREEN}[✓] ViewDNS Historical IPs:{Colors.END}")
        for rec in viewdns_history[:5]:
            print(f"    ├─ {rec['ip']} - {rec['last_seen']} ({rec['owner'][:30]})")
        historical_ips.extend(viewdns_history)
    
    st_history = get_securitytrails_history(domain)
    if st_history:
        print(f"{Colors.GREEN}[✓] SecurityTrails Historical IPs:{Colors.END}")
        for rec in st_history[:5]:
            print(f"    ├─ {rec['ip']} - Last seen: {rec['last_seen']}")
        historical_ips.extend(st_history)
    
    # Phase 7: Subdomain Scanning (optional)
    subdomains_found = []
    print(f"\n{Colors.CYAN}Would you like to perform subdomain scanning? (yes/no): {Colors.END}", end="")
    try:
        user_input = input().strip().lower()
        if user_input in ['yes', 'y']:
            subdomains_found = subdomain_scan(domain)
    except:
        pass
    
    # Generate Final Report
    generate_report(domain, primary_ip, real_ips, cdn_ips, ip_info, tech_stack, 
                   whois_data, security_headers_result, historical_ips, subdomains_found, cdn_providers)

if __name__ == "__main__":
    main()
