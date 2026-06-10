# Red Team Reference

## Overview

Red teaming is a security assessment methodology where skilled professionals simulate real-world attacks to test an organization's security defenses, detection capabilities, and response procedures. The goal is to identify vulnerabilities and improve security posture through adversarial simulation.

## Historical Context

The term "red team" originates from military war games where the "red team" played the role of enemy forces, while the "blue team" defended. This adversarial approach was adopted by cybersecurity to test defenses realistically.

## Core Principles

### 1. Adversary Emulation
- Think and act like real attackers
- Use actual attack tools and techniques
- Follow realistic attack paths
- Adapt based on defensive responses

### 2. Authorized Testing
- Always have explicit authorization
- Follow rules of engagement (ROE)
- Respect scope and boundaries
- Maintain ethics and professionalism

### 3. Objective Focused
- Test specific security capabilities
- Measure detection and response
- Identify gaps in defenses
- Provide actionable recommendations

### 4. Continuous Improvement
- Use findings to improve defenses
- Share lessons learned
- Update security controls
- Retest to verify improvements

## Red Team Methodology

### Phase 1: Planning & Preparation
1. **Define objectives**: What security capabilities to test?
2. **Establish scope**: What systems, networks, people are in scope?
3. **Set rules of engagement**: What actions are allowed/prohibited?
4. **Obtain authorization**: Written approval from stakeholders
5. **Plan communication**: How to report findings, emergencies?

### Phase 2: Reconnaissance
1. **Passive reconnaissance**: Gather information without interacting
2. **Active reconnaissance**: Interact with target to gather info
3. **OSINT (Open Source Intelligence)**: Public information gathering
4. **Network mapping**: Discover systems and services
5. **Vulnerability identification**: Find potential weaknesses

### Phase 3: Initial Compromise
1. **Exploitation**: Use vulnerabilities to gain access
2. **Social engineering**: Manipulate people to gain access
3. **Physical attacks**: Test physical security controls
4. **Wireless attacks**: Test wireless network security
5. **Web application attacks**: Test web security

### Phase 4: Post-Compromise
1. **Privilege escalation**: Gain higher level access
2. **Lateral movement**: Move through the network
3. **Persistence**: Maintain access over time
4. **Data collection**: Gather sensitive information
5. **Exfiltration**: Remove data from environment

### Phase 5: Reporting & Remediation
1. **Document findings**: Detailed evidence and methodology
2. **Assess impact**: Business and security impact
3. **Provide recommendations**: Actionable remediation steps
4. **Debrief stakeholders**: Present findings and lessons
5. **Retest fixes**: Verify remediation effectiveness

## Attack Techniques by Category

### Initial Access:
- **Phishing**: Deceptive emails to gain credentials
- **Vulnerability exploitation**: Software bugs to gain access
- **Password attacks**: Cracking, spraying, reusing credentials
- **Supply chain attacks**: Compromise through vendors/partners
- **Physical attacks**: USB drops, tailgating, lock picking

### Execution:
- **Command injection**: Execute commands on systems
- **Script execution**: Run malicious scripts
- **API exploitation**: Abuse legitimate functionality
- **Scheduled tasks**: Persistence through automation
- **Memory injection**: Run code in process memory

### Persistence:
- **Scheduled tasks**: Regularly execute malicious code
- **Service installation**: Install persistent services
- **Registry modifications**: Auto-start programs
- **Browser extensions**: Persistent web access
- **Backdoors**: Hidden access mechanisms

### Privilege Escalation:
- **Kernel exploits**: Operating system vulnerabilities
- **Service misconfigurations**: Abuse legitimate privileges
- **Token manipulation**: Abuse authentication tokens
- **Credential dumping**: Extract passwords from memory
- **Sudo/SUID abuse**: Misuse of privilege mechanisms

### Defense Evasion:
- **Obfuscation**: Hide malicious activity
- **Log manipulation**: Remove evidence from logs
- **File deletion**: Remove malicious files
- **Process hiding**: Hide malicious processes
- **Network hiding**: Use encrypted/obscured channels

### Credential Access:
- **Credential dumping**: Extract from memory/files
- **Keylogging**: Capture keyboard input
- **Credential phishing**: Trick users into revealing
- **Brute force**: Guess passwords systematically
- **Password reuse**: Try known passwords elsewhere

### Discovery:
- **Network scanning**: Map systems and services
- **Account enumeration**: Discover user accounts
- **Permission discovery**: Understand access rights
- **System information**: Gather system details
- **Network share discovery**: Find file shares

### Lateral Movement:
- **Pass the hash**: Use hashed credentials
- **Pass the ticket**: Use Kerberos tickets
- **Remote services**: Use legitimate remote access
- **Software deployment**: Use management tools
- **Exploitation**: Move using vulnerabilities

### Collection:
- **Screen capture**: Capture screenshots
- **Keylogging**: Record keystrokes
- **Clipboard capture**: Get copied data
- **Microphone/camera**: Audio/video capture
- **Data staging**: Gather data for exfiltration

### Exfiltration:
- **DNS tunneling**: Hide data in DNS queries
- **HTTP/HTTPS**: Hide data in web traffic
- **Email**: Send data via email
- **Cloud storage**: Upload to cloud services
- **Physical removal**: USB drives, printed documents

## Rules of Engagement (ROE)

### Must Include:
- **Authorization scope**: What systems/networks/people
- **Time restrictions**: When testing can occur
- **Allowed techniques**: What attack methods permitted
- **Prohibited actions**: What must not be done
- **Emergency contacts**: Who to contact if issues arise
- **Communication methods**: How to communicate during test
- **Success criteria**: How success is measured

### Common Restrictions:
- **No production impact**: Cannot disrupt business operations
- **No data modification**: Cannot alter or delete data
- **No Denial of Service**: Cannot cause service outages
- **Limited social engineering**: Restrictions on who/how to target
- **No physical damage**: Cannot cause physical harm or damage
- **Legal compliance**: Must comply with all laws/regulations

## Tools and Frameworks

### Reconnaissance:
- **Nmap**: Network discovery and port scanning
- **Shodan**: Search engine for Internet-connected devices
- **theHarvester**: Email, subdomain, and name gathering
- **Maltego**: Open source intelligence and link analysis
- **Recon-ng**: Web reconnaissance framework

### Vulnerability Scanning:
- **Nessus**: Comprehensive vulnerability scanner
- **OpenVAS**: Open source vulnerability scanner
- **Nexpose**: Risk-based vulnerability management
- **Qualys**: Cloud-based vulnerability management
- **Burp Suite**: Web application security testing

### Exploitation:
- **Metasploit**: Exploitation framework
- **Cobalt Strike**: Adversary simulation platform
- **Empire**: Post-exploitation framework
- **PowerSploit**: PowerShell-based exploitation
- **SQLmap**: Automated SQL injection tool

### Post-Exploitation:
- **Mimikatz**: Credential extraction tool
- **BloodHound**: Active Directory analysis
- **PowerView**: PowerShell AD reconnaissance
- **PsExec**: Execute processes on remote systems
- **Responder**: LLMNR/NBT-NS/mDNS poisoner

### Command and Control (C2):
- **Cobalt Strike**: Feature-rich C2 platform
- **Empire**: PowerShell C2 framework
- **Merlin**: Cross-platform C2
- **Sliver**: Open source C2 framework
- **Metasploit**: Meterpreter C2 capabilities

## Reporting and Metrics

### Key Metrics:
- **Time to Compromise**: How long to gain initial access
- **Time to Privilege Escalation**: How long to gain admin rights
- **Time to Lateral Movement**: How long to move between systems
- **Time to Critical Asset**: How long to reach most sensitive data
- **Detection Rate**: Percentage of activities detected
- **Mean Time to Detect (MTTD)**: Average time to detection
- **Mean Time to Respond (MTTR)**: Average time to response
- **Containment Effectiveness**: How well attacks were contained

### Report Structure:
1. **Executive Summary**: High-level findings and recommendations
2. **Methodology**: Approach and techniques used
3. **Findings**: Detailed vulnerabilities and evidence
4. **Impact Assessment**: Business and security impact
5. **Recommendations**: Actionable remediation steps
6. **Appendices**: Technical details, evidence, tools used

## Integration with Other Skills

### With White Hat:
- **Red team**: Finds vulnerabilities through attack simulation
- **White hat**: Builds defenses based on findings
- **Combination**: Comprehensive security improvement cycle

### With Devil's Advocate:
- **Red team**: Security-focused adversarial thinking
- **Devil's advocate**: General logical challenge
- **Combination**: Security + logical comprehensive challenge

### With Assumption Buster:
- **Red team**: Tests security assumptions through attack
- **Assumption buster**: General assumption falsification
- **Combination**: Security assumption testing + general assumption testing

### With Trust But Verify:
- **Red team**: Tests security claims through attack
- **Trust but verify**: Independently validates claims
- **Combination**: Attack-based verification + general verification

## Ethical Considerations

### Professional Ethics:
- **Authorization**: Never test without permission
- **Scope**: Stay within authorized boundaries
- **Disclosure**: Report all findings to authorized parties
- **Confidentiality**: Protect discovered information
- **Responsibility**: Use skills to improve security, not harm

### Legal Compliance:
- **Computer Fraud and Abuse Act (CFAA)**: US computer crime law
- **GDPR**: European data protection regulation
- **HIPAA**: US healthcare privacy law
- **PCI-DSS**: Payment card industry security standard
- **Local laws**: Jurisdiction-specific regulations

### Responsible Disclosure:
- **Report to organization**: Primary responsibility
- **Allow time for remediation**: Reasonable time to fix
- **Coordinate public disclosure**: If necessary, coordinate timing
- **Protect users**: Minimize risk to users during disclosure
- **Follow established processes**: Use organization's disclosure process

## Resources

### Further Reading:
- "The Cuckoo's Egg" by Cliff Stoll (true story of early red teaming)
- "Ghost in the Wires" by Kevin Mitnick (social engineering insights)
- "The Web Application Hacker's Handbook" (technical reference)
- "Red Team Field Manual" (RTFM) by Ben Clark (quick reference)

### Certifications:
- **CRT (Certified Red Teamer)**: Zero Point Security
- **OSCP (Offensive Security Certified Professional)**: Offensive Security
- **GPEN (GIAC Penetration Tester)**: GIAC/SANS
- **CEH (Certified Ethical Hacker)**: EC-Council
- **eJPT (eLearnSecurity Junior Penetration Tester)**: eLearnSecurity

### Communities and Resources:
- **Reddit**: /r/netsec, /r/redteamsec
- **Twitter**: #redteam, #infosec communities
- **Conferences**: DEF CON, Black Hat, BSides
- **Training**: HackTheBox, TryHackMe, Pentester Academy

*Last updated: 2026-02-26*