# Advanced DDoS Methods — Research Review (2025–2026)

**Purpose:** Literature and industry survey to inform the SH-MVDoS lab framework.  
**Scope:** Authorized defensive research only — vectors are modeled for isolated lab environments.  
**Date:** June 2026

---

## Executive Summary

DDoS in 2025–2026 is characterized by **hyper-volumetric network floods**, **HTTP/2 protocol exploitation**, **API-centric L7 abuse**, **QUIC transport exhaustion**, and emerging **AI-adaptive attack patterns**. Industry telemetry shows attacks more than doubled YoY; academic work focuses on evasion against ML/rule detectors and adversarial-robust classification.

SH-MVDoS should prioritize a **multi-vector lab catalog** aligned with these trends, with explicit coupling to **defense evaluation** (detection latency, mitigation effectiveness, resource exhaustion metrics).

---

## 1. Industry Threat Landscape (2025–2026)

### 1.1 Cloudflare Q4 2025 DDoS Threat Report

**Source:** [Cloudflare DDoS Threat Report 2025 Q4](https://blog.cloudflare.com/ddos-threat-report-2025-q4/) (Feb 2026)

| Metric | Value |
|--------|-------|
| Total DDoS attacks (2025) | 47.1M (+121% YoY) |
| Peak volumetric attack | **31.4 Tbps** (35 seconds) |
| Peak HTTP flood | **205 Mrps** |
| Network-layer share (Q4) | 78% of all attacks |
| Hyper-volumetric growth | +700% vs late 2024 |

**Key campaigns:**
- **Aisuru-Kimwolf botnet** — ~1–4M infected Android TVs; "Night Before Christmas" campaign (Dec 2025) with 20–205 Mrps HTTP floods.
- **Multi-vector Q1 2025** — 18-day campaign: SYN flood, Mirai, SSDP amplification.
- **HTTP/2 Rapid Reset resurgence** — L7 attack sizes returned to 2023 Rapid Reset levels.

### 1.2 Akamai State of the Internet 2026 (API / App DDoS)

**Source:** [Akamai SOTI App & API DDoS Security Report 2026](https://www.akamai.com/lp/soti/app-api-ddos-security-report-2026)

- APIs are the #1 attack surface — 87% of organizations experienced API security incidents in 2025.
- L7 DDoS +104% over two years; campaigns are multi-vector and industrialized.

### 1.3 Imperva Early 2025 Trends

**Source:** [Imperva Early 2025 DDoS Trends](https://www.imperva.com/blog/early-2025-ddos-attacks-signal-a-dangerous-trend-in-cybersecurity/)

Application-layer attacks are increasingly protocol-aware; botnet sourcing favors cloud VMs.

---

## 2. Protocol-Layer Attacks

### 2.1 HTTP/2 Rapid Reset — CVE-2023-44487

Opens many streams, sends headers, immediately RST_STREAM — exhausts server stream tracking.

- [Cloudflare breakdown](https://blog.cloudflare.com/technical-breakdown-http2-rapid-reset-ddos-attack/)
- [CERT VU#767506](https://kb.cert.org/vuls/id/767506)

### 2.2 MadeYouReset (2025)

Protocol-compliant malformed frames (WINDOW_UPDATE abuse, half-closed streams) cause **server self-reset**, bypassing client RST rate limits.

- [Imperva](https://www.imperva.com/blog/madeyoureset-turning-http-2-server-against-itself/)
- [Cloudflare mitigations](https://blog.cloudflare.com/madeyoureset-an-http-2-vulnerability-thwarted-by-rapid-reset-mitigations/)

### 2.3 HTTP/2 Bomb — CVE-2026-49975

HPACK / CONTINUATION frame bombs expanding server-side processing.

### 2.4 React2DoS — CVE-2026-23869

RSC Flight protocol abuse against Next.js lab victims.

---

## 3. QUIC / HTTP/3

| Topic | Source |
|-------|--------|
| Connection ID exhaustion | [seemann.io](https://seemann.io/posts/2024-03-19---exploiting-quics-connection-id-management/) |
| QUIC Leak CVE-2025-54939 | [Imperva](https://www.imperva.com/blog/quic-leak-cve-2025-54939-new-high-risk-pre-handshake-remote-denial-of-service-in-lsquic-quic-implementation/) |
| State exhaustion taxonomy | [FastNetMon](https://fastnetmon.com/2025/08/12/understanding-transport-and-state-exhaustion-ddos-attacks/) |

---

## 4. API / L7 Layer

- **GraphQL depth/complexity** — [Cloudflare](https://blog.cloudflare.com/protecting-graphql-apis-from-malicious-queries/), [Checkmarx](https://checkmarx.com/blog/exploiting-graphql-query-depth/)
- **Slowloris / slow POST** — connection exhaustion
- **Framework abuse** — WordPress Heartbeat, Magento guest-cart, Next.js image proxy

---

## 5. Adaptive / AI-Driven (Academic)

### AdaDoS — arXiv:2510.20566 (Oct 2025)

RL-based adaptive DoS evading ML/rule SDN detectors. Two-stage PPO: attack timing + rate shaping. Teacher-student learning under partial observability (ping RTT only).

**SH-MVDoS role:** Defense benchmark / adaptive probe mode — not production attack tooling.

### 3D CNN DDoS Classification — arXiv:2509.10543

Adversarial-robust traffic classification for defender evaluation.

---

## 6. SH-MVDoS v2 Vector Catalog

| Priority | Vector | Layer | Victim |
|----------|--------|-------|--------|
| P0 | h2-rapid-reset | L7 | Nginx HTTP/2 |
| P0 | l7-abuser | L7 | Any HTTP |
| P1 | h2-madeyoureset | L7 | Nginx HTTP/2 |
| P1 | quic-burner | L4/7 | Nginx HTTP/3 |
| P1 | graphql-depth | L7 | GraphQL victim |
| P2 | slowloris | L7 | Any HTTP |
| P3 | adaptive-probe | Meta | RTT-driven |

**Modes:** manual | hybrid | auto | adaptive  
**Ethics:** allowlist-only targets, watchdog off by default, rate caps in runtime.env.

---

## References

1. Cloudflare Q4 2025 DDoS Report — https://blog.cloudflare.com/ddos-threat-report-2025-q4/
2. Akamai SOTI 2026 — https://www.akamai.com/lp/soti/app-api-ddos-security-report-2026
3. AdaDoS — https://arxiv.org/abs/2510.20566
4. MadeYouReset — https://www.imperva.com/blog/madeyoureset-turning-http-2-server-against-itself/
5. QUIC CID — https://seemann.io/posts/2024-03-19---exploiting-quics-connection-id-management/