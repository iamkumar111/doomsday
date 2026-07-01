# CloudflareBypass

**Enterprise-Grade CDN Reconnaissance & Origin IP Discovery Engine**

[![Python 3.8+](https://img.shields.io/badge/Python-3.8%2B-blue.svg)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Maintenance](https://img.shields.io/badge/Maintained-Yes-brightgreen.svg)]()

---

## 📖 Overview

**CloudflareBypass** is a specialized security reconnaissance suite designed to identify the backend infrastructure of websites protected by Content Delivery Networks (CDNs) like Cloudflare, Akamai, and Fastly. 

By leveraging multi-layer DNS leakage analysis, Certificate Transparency (CT) logs, and direct-to-IP technology fingerprinting, this suite provides security researchers with a way to discover Origin IPs and analyze the underlying technology stack without triggering standard WAF alerts.

## ✨ Key Features

- 🔍 **Origin IP Discovery**: Multi-vector analysis using DNS, CT Logs, and BufferOver.
- 🧩 **Stack Fingerprinting**: Automatic detection of Backend Languages (PHP, Node.js, Python), Frameworks (React, Vue, Next.js), and CMS (WordPress, Joomla).
- 🤫 **Silent Recon**: "Zero-trace" scanning mode to gather intelligence without active engagement.
- 📊 **Security Scoring**: Automatic analysis of security headers (HSTS, CSP, XSS protection).
- 🌍 **Geo-Intelligence**: Detailed ISP, ASN, and Geolocation data for target servers.

---

## 🛠️ Installation & Setup

### Environment Preparation
It is highly recommended to use a Python Virtual Environment to maintain system stability.

```bash
# Clone the repository
git clone https://github.com/yourusername/CloudflareBypass.git
cd CloudflareBypass

# Initialize Virtual Environment
python3 -m venv .venv
source .venv/bin/activate  # Windows: .venv\Scripts\activate

# Install Core Dependencies
pip install -r requirements.txt
```

### System-Wide Installation (Alternative)
If you prefer system-wide installation on modern Linux distributions:
```bash
pip install -r requirements.txt --break-system-packages
```

---

## 🚀 Execution Guide

### 1. Advanced Intelligence Scan (GODINTEL)
Deep analysis of domain infrastructure and technology stack.
```bash
python3 godintelv11.py <domain.com>
```

### 2. Stealth Protocol (SHADOWRECON)
Ideal for passive information gathering.
```bash
python3 zerotraceip.py <domain.com>
```

### 3. Rapid Origin Finder (REALIP)
High-speed script for targeted IP resolution.
```bash
# Update the TARGET variable inside the script first
python3 detect_realip_fast.py
```

---

## 👨‍💻 Author & Contact

**Dimpal Sharma**  
*DevSecOps Engineer | Cloud Security Specialist*  
*Specializing in Infrastructure Security & Professional Reconnaissance Tools*

- **Email**: [sharamhu16@gmail.com](mailto:sharamhu16@gmail.com)
- **Phone**: +91 7976327138

---

## 📜 License & Legal
This project is licensed under the **MIT License**.

> [!WARNING]
> This tool is intended only for **authorized security research** and educational purposes. Unauthorized use against systems without prior consent is illegal and strictly prohibited. The author assumes no liability for misuse of this software.
