#!/bin/bash
set -e

echo "Red Team Reconnaissance" >&2
echo "======================" >&2

# Check for required arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <target> [scope] [intensity]" >&2
    echo "Example: $0 example.com external medium" >&2
    echo "Scope: external, internal, limited, full" >&2
    echo "Intensity: light, medium, aggressive, stealth" >&2
    exit 1
fi

TARGET="$1"
SCOPE="${2:-external}"
INTENSITY="${3:-medium}"
TIMESTAMP=$(date +%Y-%m-%dT%H:%M:%S)

echo "Target: $TARGET" >&2
echo "Scope: $SCOPE" >&2
echo "Intensity: $INTENSITY" >&2
echo "Timestamp: $TIMESTAMP" >&2
echo "" >&2

# Generate reconnaissance plan
echo "Generating reconnaissance plan..." >&2
echo "" >&2

cat << EOF
Red Team Reconnaissance Plan
──────────────────────────────
Target: $TARGET
Scope: $SCOPE
Intensity: $INTENSITY
Phase: Reconnaissance
Date: $TIMESTAMP

WARNING: Only conduct reconnaissance with proper authorization
and within established rules of engagement.

Reconnaissance Methodology:

EOF

case "$SCOPE" in
    "external")
        echo "External Reconnaissance (Internet-facing):" >&2
        cat << EOF
1. Passive Information Gathering:
   - DNS records (A, AAAA, MX, TXT, NS, SOA)
   - Subdomain enumeration
   - WHOIS information
   - SSL/TLS certificate information
   - Search engine results (Google dorking)
   - Social media presence
   - Job postings (revealing technologies)
   - GitHub/GitLab repositories (code leaks)

2. Active Information Gathering:
   - Port scanning (top 1000 ports)
   - Service version detection
   - Web application enumeration
   - Directory/file brute forcing
   - Technology fingerprinting
   - Email address harvesting
   - Network topology mapping

3. Vulnerability Identification:
   - Common service vulnerabilities
   - Web application security headers
   - SSL/TLS configuration weaknesses
   - Open ports with known vulnerabilities
   - Default credentials on exposed services
EOF
        ;;
    "internal")
        echo "Internal Reconnaissance (Network access gained):" >&2
        cat << EOF
1. Network Discovery:
   - ARP scanning
   - NetBIOS enumeration
   - SNMP enumeration
   - LDAP/Active Directory reconnaissance
   - Share enumeration (SMB, NFS)
   - Network topology mapping

2. Host Discovery:
   - Operating system fingerprinting
   - User account enumeration
   - Service enumeration
   - Installed software inventory
   - Patch level assessment
   - Configuration review

3. Privilege and Trust Mapping:
   - User privilege levels
   - Group memberships
   - Service accounts
   - Trust relationships between systems
   - Authentication mechanisms
EOF
        ;;
    "limited")
        echo "Limited Reconnaissance (Targeted):" >&2
        cat << EOF
1. Focused Information Gathering:
   - Specific subdomains only
   - Limited port range scanning
   - Targeted service enumeration
   - Minimal footprint operations

2. Stealth Techniques:
   - Slow scanning to avoid detection
   - Distributed scanning sources
   - Non-standard ports only
   - Encrypted channels
   - Legitimate-looking traffic patterns
EOF
        ;;
    "full")
        echo "Full Reconnaissance (Comprehensive):" >&2
        cat << EOF
1. Comprehensive External + Internal:
   - All external reconnaissance techniques
   - All internal reconnaissance techniques
   - Physical location reconnaissance
   - Social engineering reconnaissance
   - Supply chain reconnaissance

2. Advanced Techniques:
   - Wireless network reconnaissance
   - Bluetooth device discovery
   - RFID/NFC reconnaissance
   - Physical security assessment
   - Personnel reconnaissance
EOF
        ;;
    *)
        echo "Unknown scope: $SCOPE" >&2
        cat << EOF
- Using external scope by default
EOF
        ;;
esac

cat << EOF

Intensity-Specific Techniques:

EOF

case "$INTENSITY" in
    "light")
        echo "Light intensity (Low detection risk):" >&2
        cat << EOF
- Public information only (no active scanning)
- Rate-limited requests
- Using legitimate-looking user agents
- Avoiding aggressive enumeration
- Short duration sessions
EOF
        ;;
    "medium")
        echo "Medium intensity (Balanced approach):" >&2
        cat << EOF
- Standard scanning techniques
- Moderate rate limits
- Common user agents
- Standard enumeration
- Business hours activity
EOF
        ;;
    "aggressive")
        echo "Aggressive intensity (Maximum information):" >&2
        cat << EOF
- Comprehensive scanning
- High request rates
- Multiple user agents
- Deep enumeration
- 24/7 activity
EOF
        ;;
    "stealth")
        echo "Stealth intensity (Avoid detection):" >&2
        cat << EOF
- Slow, distributed scanning
- Legitimate traffic patterns
- Encrypted channels
- Avoiding known detection signatures
- Mimicking legitimate users
- Long timeframes (weeks/months)
EOF
        ;;
    *)
        echo "Unknown intensity: $INTENSITY" >&2
        cat << EOF
- Using medium intensity by default
EOF
        ;;
esac

cat << EOF

Tools and Commands:

1. DNS Reconnaissance:
   dig $TARGET ANY
   dig @8.8.8.8 $TARGET AXFR
   nslookup -type=any $TARGET
   dnsrecon -d $TARGET

2. Subdomain Enumeration:
   sublist3r -d $TARGET
   amass enum -d $TARGET
   knockpy $TARGET
   crt.sh (Certificate Transparency logs)

3. Port Scanning:
   nmap -sS -sV -O -p- $TARGET
   masscan -p0-65535 $TARGET --rate=1000
   rustscan -a $TARGET -- -sV -sC -O

4. Web Application Recon:
   dirb http://$TARGET
   nikto -h http://$TARGET
   whatweb http://$TARGET
   wfuzz -c -z file,wordlist/general/common.txt http://$TARGET/FUZZ

5. Vulnerability Scanning:
   nuclei -u http://$TARGET
   gitleaks --repo-url=https://github.com/$TARGET
   sslscan $TARGET

Detection Risk Assessment:
- IDS/IPS evasion required: [Yes/No]
- WAF presence likely: [Yes/No]
- Logging comprehensiveness: [Low/Medium/High]
- Alerting sensitivity: [Low/Medium/High]
- Response capability: [Limited/Moderate/Advanced]

Rules of Engagement Checklist:
✅ Authorization obtained
✅ Scope clearly defined
✅ Time restrictions understood
✅ Emergency contacts documented
✅ Communication plan established
✅ Success criteria defined
✅ Legal compliance verified

Documentation Template:
1. Target: $TARGET
2. Scope: $SCOPE
3. Intensity: $INTENSITY
4. Date: $TIMESTAMP
5. Findings: [DNS records, open ports, services, technologies]
6. Vulnerabilities identified: [List]
7. Attack surface mapped: [Summary]
8. Next phase recommendations: [Initial access vectors]
9. Detection risk: [Assessment]
10. Authorization evidence: [Reference]

Next Phase Planning:
Based on reconnaissance findings, prepare for:
- Initial access attempts
- Vulnerability exploitation
- Social engineering campaigns
- Physical access attempts (if in scope)

EOF

echo '{"status": "plan_generated", "target": "'"$TARGET"'", "scope": "'"$SCOPE"'", "intensity": "'"$INTENSITY"'", "timestamp": "'"$TIMESTAMP"'", "phase": "reconnaissance", "authorization_required": true}'