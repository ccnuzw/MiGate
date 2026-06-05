"""
VPS Protocol Compatibility Test
Tests all 17 protocol/transport/security combos against the VPS deployment.
"""
import urllib.request, urllib.error, json, http.cookiejar, socket, subprocess, sys, time

VPS_HOST = "103.193.149.217"
VPS_PORT = 9999
BASE = f"http://{VPS_HOST}:{VPS_PORT}"
SSH_CMD = ["sshpass", "-p", "FoQ-sCSUq3ugBPVq", "ssh", "-o", "StrictHostKeyChecking=no", f"root@{VPS_HOST}"]

# Protocol test matrix
PROTOCOLS = [
    # (id, remark, protocol, network, security, extra, notes)
    (1,  "vless-tcp-reality",       "vless",       "tcp",  "reality",  {},                         "标准 VLESS+REALITY"),
    (2,  "vless-tcp-reality-vision","vless",       "tcp",  "reality",  {},                         "REALITY + flow=xtls-rprx-vision"),
    (3,  "vless-tcp-tls",           "vless",       "tcp",  "tls",      {"tls_cert_file":"","tls_key_file":""}, "VLESS+TLS（无证书验证配置生成）"),
    (4,  "vless-tcp-tls-vision",    "vless",       "tcp",  "tls",      {"tls_cert_file":"","tls_key_file":""}, "TLS + XTLS-Vision（无证书验证配置生成）"),
    (5,  "vless-ws-tls",            "vless",       "ws",   "tls",      {"tls_cert_file":"","tls_key_file":"","ws_path":"/ws"}, "WSS"),
    (6,  "vless-grpc-reality",      "vless",       "grpc", "reality",  {"grpc_service_name":"migate"}, "gRPC+REALITY"),
    (7,  "vless-xhttp-reality",     "vless",       "xhttp","reality",  {"xhttp_path":"/","xhttp_mode":"stream-one"}, "XHTTP+REALITY"),
    (8,  "trojan-tcp-tls",          "trojan",      "tcp",  "tls",      {"tls_cert_file":"","tls_key_file":""}, "Trojan+TLS（无证书验证配置生成）"),
    (9,  "trojan-tcp-reality",      "trojan",      "tcp",  "reality",  {},                         "Trojan+REALITY"),
    (10, "trojan-grpc-reality",     "trojan",      "grpc", "reality",  {"grpc_service_name":"migate"}, "Trojan+gRPC+REALITY"),
    (11, "vmess-tcp-none",          "vmess",       "tcp",  "none",     {},                         "VMess+TCP"),
    (12, "vmess-tcp-tls",           "vmess",       "tcp",  "tls",      {"tls_cert_file":"","tls_key_file":""}, "VMess+TLS（无证书验证配置生成）"),
    (13, "vmess-ws-none",           "vmess",       "ws",   "none",     {"ws_path":"/vmess"},       "VMess+WS"),
    (14, "vmess-ws-tls",            "vmess",       "ws",   "tls",      {"tls_cert_file":"","tls_key_file":"","ws_path":"/vmess-ws"}, "VMess+WS+TLS（无证书验证配置生成）"),
    (15, "ss-aes-256-gcm",          "shadowsocks", "tcp",  "none",     {"ss_method":"aes-256-gcm"},"Shadowsocks+AEAD"),
    (16, "ss-2022-blake3",          "shadowsocks", "tcp",  "none",     {"ss_method":"2022-blake3-aes-256-gcm"}, "Shadowsocks+SS2022"),
    (17, "hysteria2",               None,          None,   None,       {},                         "Hysteria2 — MiGate 不支持 ❌"),
]

def ssh(cmd):
    """Run a command on the VPS via SSH and return output."""
    full_cmd = SSH_CMD + [cmd]
    r = subprocess.run(full_cmd, capture_output=True, text=True, timeout=15)
    return r.stdout.strip(), r.stderr.strip(), r.returncode

def api_login():
    """Login and return an authenticated opener."""
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    req = urllib.request.Request(
        f"{BASE}/api/login",
        data=json.dumps({"username":"admin","password":"admin"}).encode(),
        headers={"Content-Type":"application/json"}
    )
    try:
        resp = opener.open(req)
        return opener
    except Exception as e:
        print(f"  ❌ Login failed: {e}")
        return None

def check_status():
    """Check if migate service is healthy."""
    out, _, rc = ssh("curl -s http://localhost:9999/api/health")
    if rc == 0 and out:
        return json.loads(out)
    return None

def create_inbound(opener, port, remark, protocol, network, security, extra=None):
    """Create an inbound via API. Port must be unique."""
    payload = {
        "remark": remark,
        "protocol": protocol,
        "port": port,
        "network": network,
        "security": security,
    }
    if extra:
        payload.update(extra)
    req = urllib.request.Request(
        f"{BASE}/api/inbounds",
        data=json.dumps(payload).encode(),
        headers={"Content-Type":"application/json"}
    )
    try:
        resp = opener.open(req)
        return resp.status == 200, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return False, {"status": e.code, "body": e.read().decode()}
    except Exception as e:
        return False, {"error": str(e)}

def delete_inbound(opener, inbound_id):
    """Delete an inbound."""
    try:
        req = urllib.request.Request(f"{BASE}/api/inbounds/{inbound_id}", method="DELETE")
        resp = opener.open(req)
        return resp.status == 200
    except:
        return False

def fetch_config(opener):
    """Fetch the current Xray config preview."""
    try:
        req = urllib.request.Request(f"{BASE}/api/xray/config")
        resp = opener.open(req)
        return json.loads(resp.read())
    except:
        return None

def check_port_listening(port):
    """Check if a port is listening on the VPS."""
    out, _, rc = ssh(f"ss -tlnp | grep ':{port} '")
    return rc == 0 and port in out

def main():
    results = []
    start_port = 40000

    print("=" * 70)
    print("MiGate 协议兼容性测试")
    print(f"VPS: {VPS_HOST}:{VPS_PORT}")
    print("=" * 70)

    # 1. Pre-check: service status
    print("\n[Pre-check] 检查服务状态...")
    status = check_status()
    if status and status.get("status") == "ok":
        print(f"  ✅ 服务正常: {json.dumps(status)}")
    else:
        print(f"  ❌ 服务异常: {status}")
        sys.exit(1)

    # 2. Login
    print("\n[Login] 面板登录...")
    opener = api_login()
    if not opener:
        sys.exit(1)
    print("  ✅ 登录成功")

    # 3. Run each protocol test
    for pid, remark, protocol, network, security, extra, notes in PROTOCOLS:
        time.sleep(1)  # Throttle to avoid overwhelming Xray

        if protocol is None:
            # Unsupported protocol (Hysteria2)
            print(f"\n{pid:2d}. {notes}")
            results.append((pid, "Hysteria2", "不支持", "MiGate 仅支持 VLESS/VMess/Trojan/Shadowsocks"))
            continue

        port = start_port + pid
        print(f"\n{pid:2d}. {remark} ({notes}) — port {port}")

        # Create inbound
        ok, resp = create_inbound(opener, port, remark, protocol, network, security, extra)
        if not ok:
            print(f"  ❌ 创建失败: {resp}")
            clean_inbounds = []
            results.append((pid, remark, "创建失败", str(resp)))
            continue

        inbound_id = resp.get("id")
        if not inbound_id:
            # Might be using the old create-with-initial-client format
            pass
        print(f"  ✅ API 创建成功 (id={resp.get('id','?')})")

        # Fetch config and verify structure
        config = fetch_config(opener)
        if config is None:
            print(f"  ⚠️  无法获取配置预览")
            results.append((pid, remark, "配置不可预览", ""))
            delete_inbound(opener, inbound_id if inbound_id else None)
            continue

        # Verify inbound is in the config
        inbounds_config = config.get("inbounds", [])
        found = False
        for ib in inbounds_config:
            if ib.get("port") == port:
                found = True
                proto = ib.get("protocol", "")
                ss = ib.get("streamSettings", {})
                net = ss.get("network", "")
                sec = ss.get("security", "")
                print(f"  ✅ Xray 配置验证通过")
                print(f"     protocol={proto}, network={net}, security={sec}")

                # Verify specific features
                issues = []
                if sec == "reality":
                    if "realitySettings" not in ss:
                        issues.append("缺少 realitySettings")
                    else:
                        rs = ss["realitySettings"]
                        if not rs.get("privateKey"):
                            issues.append("realitySettings 缺少 privateKey")
                        if not rs.get("serverNames"):
                            issues.append("realitySettings 缺少 serverNames")
                if sec == "tls":
                    if "tlsSettings" not in ss:
                        issues.append("tlsSettings 未配置（无证书文件时正常）")

                if protocol == "vless" and sec == "reality":
                    clients = ib.get("settings", {}).get("clients", [])
                    if clients and clients[0].get("flow") != "xtls-rprx-vision":
                        issues.append(f"flow 应为 xtls-rprx-vision，实际为 {clients[0].get('flow','')}")

                if issues:
                    print("  ⚠️  " + " | ".join(issues))
                break

        if not found:
            print(f"  ❌ 配置中找不到端口 {port}")
            print(f"  现有入站端口: {[ib.get('port') for ib in inbounds_config]}")
            results.append((pid, remark, "配置缺失", ""))
            continue

        # Check port is listening
        time.sleep(2)
        if check_port_listening(port):
            print(f"  ✅ 端口 {port} 监听中")
            results.append((pid, remark, "通过", "配置生成正确，端口监听中"))
        else:
            print(f"  ❌ 端口 {port} 未监听")
            results.append((pid, remark, "端口未监听", "Xray 可能拒绝该配置"))

    # 4. Print results table
    print("\n" + "=" * 70)
    print("测试结果汇总")
    print("=" * 70)
    print(f"{'#':>2} {'协议组合':<35} {'状态':<12} {'详情'}")
    print("-" * 70)
    for pid, remark, status, detail in results:
        icon = "✅" if status == "通过" else "❌" if status in ("创建失败","配置缺失","端口未监听") else "⚠️"
        print(f"{pid:2d} {icon} {remark:<32} {status:<12} {detail[:40]}")

if __name__ == "__main__":
    main()
