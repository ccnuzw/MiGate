"""
VPS Protocol Compatibility Test v2 — debugged
"""
import urllib.request, urllib.error, json, http.cookiejar, time, subprocess, sys

VPS_HOST = "103.193.149.217"
VPS_PORT = 9999
BASE = f"http://{VPS_HOST}:{VPS_PORT}"
SSH_CMD = ["sshpass", "-p", "FoQ-sCSUq3ugBPVq", "ssh", "-o", "StrictHostKeyChecking=no", f"root@{VPS_HOST}"]

PROTOCOLS = [
    (1,  "vless-tcp-reality",        "vless",       "tcp",  "reality",  {"reality_dest":"www.cloudflare.com:443","reality_server_names":"www.cloudflare.com"}, "标准 VLESS+REALITY"),
    (2,  "vless-tcp-reality-vision", "vless",       "tcp",  "reality",  {}, "REALITY + flow=xtls-rprx-vision"),
    (3,  "vless-tcp-tls",            "vless",       "tcp",  "tls",      {}, "VLESS+TLS（无证书验证配置生成）"),
    (4,  "vless-tcp-tls-vision",     "vless",       "tcp",  "tls",      {}, "TLS + XTLS-Vision（无证书）"),
    (5,  "vless-ws-tls",             "vless",       "ws",   "tls",      {"ws_path":"/ws"}, "WSS"),
    (6,  "vless-grpc-reality",       "vless",       "grpc", "reality",  {"grpc_service_name":"migate"}, "gRPC+REALITY"),
    (7,  "vless-xhttp-reality",      "vless",       "xhttp","reality",  {"xhttp_path":"/","xhttp_mode":"stream-one"}, "XHTTP+REALITY"),
    (8,  "trojan-tcp-tls",           "trojan",      "tcp",  "tls",      {}, "Trojan+TLS（无证书）"),
    (9,  "trojan-tcp-reality",       "trojan",      "tcp",  "reality",  {"reality_dest":"www.cloudflare.com:443","reality_server_names":"www.cloudflare.com"}, "Trojan+REALITY"),
    (10, "trojan-grpc-reality",      "trojan",      "grpc", "reality",  {"grpc_service_name":"migate","reality_dest":"www.cloudflare.com:443","reality_server_names":"www.cloudflare.com"}, "Trojan+gRPC+REALITY"),
    (11, "vmess-tcp-none",           "vmess",       "tcp",  "none",     {}, "VMess+TCP"),
    (12, "vmess-tcp-tls",            "vmess",       "tcp",  "tls",      {}, "VMess+TLS（无证书）"),
    (13, "vmess-ws-none",            "vmess",       "ws",   "none",     {"ws_path":"/vmess"}, "VMess+WS"),
    (14, "vmess-ws-tls",             "vmess",       "ws",   "tls",      {"ws_path":"/vmess-ws"}, "VMess+WS+TLS（无证书）"),
    (15, "ss-aes-256-gcm",           "shadowsocks", "tcp",  "none",     {"ss_method":"aes-256-gcm"}, "Shadowsocks+AEAD"),
    (16, "ss-2022-blake3",           "shadowsocks", "tcp",  "none",     {"ss_method":"2022-blake3-aes-256-gcm"}, "Shadowsocks+SS2022"),
    (17, "hysteria2",                None,          None,   None,       {}, "Hysteria2 — MiGate 不支持"),
]

def ssh(cmd):
    full_cmd = SSH_CMD + [cmd]
    r = subprocess.run(full_cmd, capture_output=True, text=True, timeout=15)
    return r.stdout.strip(), r.stderr.strip(), r.returncode

def api_login():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    req = urllib.request.Request(
        f"{BASE}/api/login",
        data=json.dumps({"username":"admin","password":"admin"}).encode(),
        headers={"Content-Type":"application/json"}
    )
    resp = opener.open(req)
    return opener

def create_inbound(opener, port, remark, protocol, network, security, extra):
    payload = {
        "remark": remark, "protocol": protocol, "port": port,
        "network": network, "security": security,
    }
    payload.update(extra)
    req = urllib.request.Request(
        f"{BASE}/api/inbounds",
        data=json.dumps(payload).encode(),
        headers={"Content-Type":"application/json"}
    )
    resp = opener.open(req)
    body = json.loads(resp.read())
    return resp.status, body

def fetch_config(opener):
    req = urllib.request.Request(f"{BASE}/api/xray/config")
    resp = opener.open(req)
    return json.loads(resp.read())

def check_config_structure(config, port, protocol, network, security):
    """Verify the Xray config contains the expected inbound."""
    for ib in config.get("inbounds", []):
        if ib.get("port") != port:
            continue
        ss = ib.get("streamSettings", {})
        issues = []
        if ib.get("protocol") != protocol:
            issues.append(f"protocol={ib.get('protocol')} expected={protocol}")
        if ss.get("network") != network:
            issues.append(f"network={ss.get('network')} expected={network}")
        if ss.get("security") != security:
            issues.append(f"security={ss.get('security')} expected={security}")
        if security == "reality":
            if "realitySettings" not in ss:
                issues.append("缺少 realitySettings")
            else:
                rs = ss["realitySettings"]
                if not rs.get("privateKey"):
                    issues.append("缺少 privateKey")
                if not rs.get("serverNames"):
                    issues.append("缺少 serverNames")
                if protocol == "vless":
                    clients = ib.get("settings", {}).get("clients", [])
                    if clients and clients[0].get("flow") != "xtls-rprx-vision":
                        issues.append(f"flow={clients[0].get('flow','')} 期望 xtls-rprx-vision")
        if network == "ws":
            if "wsSettings" not in ss:
                issues.append("缺少 wsSettings")
        if network == "grpc":
            if "grpcSettings" not in ss:
                issues.append("缺少 grpcSettings")
        if network == "xhttp":
            if "xhttpSettings" not in ss:
                issues.append("缺少 xhttpSettings")
        return True, issues or ["OK"]
    return False, [f"端口 {port} 未在 config 中找到"]

def main():
    results = []
    start_port = 41000

    print("=" * 75)
    print("MiGate 协议兼容性测试 v2")
    print(f"VPS: {VPS_HOST}:{VPS_PORT}")
    print("=" * 75)

    # Pre-check service status
    print("\n[Pre-check] 服务状态...")
    out, _, rc = ssh("curl -s http://localhost:9999/api/health")
    status = json.loads(out) if rc == 0 and out else {"error": "unreachable"}
    print(f"  {'✅' if status.get('status')=='ok' else '❌'} {json.dumps(status, ensure_ascii=False)}")

    # Login
    print("\n[Login] 面板登录...")
    opener = api_login()
    print("  ✅ 登录成功")

    # Clean up any existing test inbounds from previous run (41001-41017 range)
    print("\n[Cleanup] 清理旧测试入站...")
    list_req = urllib.request.Request(f"{BASE}/api/inbounds")
    existing = json.loads(opener.open(list_req).read())
    for ib in existing:
        if 41001 <= ib.get("port", 0) <= 41017:
            del_req = urllib.request.Request(f"{BASE}/api/inbounds/{ib['id']}", method="DELETE")
            opener.open(del_req)
            print(f"  已删除 id={ib['id']} port={ib['port']} {ib.get('remark','')}")

    created_ids = []

    # Run each protocol test
    for pid, remark, protocol, network, security, extra, notes in PROTOCOLS:
        time.sleep(1)

        if protocol is None:
            print(f"\n{pid:2d}. {remark:30s} — {notes}")
            results.append((pid, remark, "⚠️ 不支持", "MiGate 不支持 Hysteria2"))
            continue

        port = start_port + pid
        print(f"\n{'─'*60}")
        print(f"#{pid:2d} {remark:30s} port={port} — {notes}")
        print(f"{'─'*60}")

        # Create inbound
        try:
            status_code, body = create_inbound(opener, port, remark, protocol, network, security, extra)
            inbound_id = body.get("id")
            created_ids.append(inbound_id)
            print(f"  ✅ API 创建 → id={inbound_id}, status={status_code}")
        except Exception as e:
            print(f"  ❌ API 创建失败: {e}")
            results.append((pid, remark, "❌ 创建失败", str(e)[:60]))
            continue

        # Verify Xray config
        time.sleep(2)
        config = fetch_config(opener)
        if not config:
            print(f"  ❌ 无法获取 Xray 配置")
            results.append((pid, remark, "❌ 配置不可读", ""))
            continue

        found, issues = check_config_structure(config, port, protocol, network, security)
        if found:
            print(f"  ✅ 配置验证: {' | '.join(issues)}")
        else:
            print(f"  ❌ {' | '.join(issues)}")
            results.append((pid, remark, "❌ 配置异常", " | ".join(issues)))
            continue

        # Check port listening
        out, _, rc = ssh(f"ss -tlnp 2>/dev/null | grep ':{port} '")
        port_ok = rc == 0 and str(port) in out
        print(f"  {'✅' if port_ok else '⚠️'} 端口 {port} {'监听中' if port_ok else '未监听'}")

        if port_ok:
            results.append((pid, remark, "✅ 通过", "配置正确 + 端口监听"))
        else:
            results.append((pid, remark, "⚠️ 端口未监听", "配置生成正确但 Xray 可能拒绝该组合"))

    # Summary
    print("\n\n" + "=" * 75)
    print("测试结果汇总")
    print("=" * 75)
    passed = sum(1 for _, _, s, _ in results if s == "✅ 通过")
    failed = sum(1 for _, _, s, _ in results if s.startswith("❌"))
    warned = sum(1 for _, _, s, _ in results if s.startswith("⚠️") or s.startswith("⚠️ 端口"))
    print(f"总计: {len(results)} | ✅ 通过: {passed} | ❌ 失败: {failed} | ⚠️ 警告/不支持: {warned}")
    print()
    for pid, remark, status, detail in results:
        icon = "✅" if status == "✅ 通过" else "❌" if status.startswith("❌") else "⚠️"
        print(f"  {pid:2d} {icon} {remark:<30s} {status:<12s} {detail[:55]}")

if __name__ == "__main__":
    main()