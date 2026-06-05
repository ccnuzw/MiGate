"""
MiGate 协议兼容性测试 v3
逐一创建各协议组合入站（带初始客户端），验证 Xray 配置和端口监听。
"""
import urllib.request, urllib.error, json, http.cookiejar, time, subprocess

VPS_HOST = "103.193.149.217"
VPS_PORT = 9999
BASE = f"http://{VPS_HOST}:{VPS_PORT}"
SSH_CMD = ["sshpass", "-p", "FoQ-sCSUq3ugBPVq", "ssh", "-o", "StrictHostKeyChecking=no", f"root@{VPS_HOST}"]

PROTOCOLS = [
    # (id, remark, protocol, network, security, extra)
    (1,  "vless-tcp-reality",        "vless",       "tcp",  "reality",  {"initial_client": {"email": "test1@t.com"}}),
    (2,  "vless-tcp-reality-vision", "vless",       "tcp",  "reality",  {"initial_client": {"email": "test2@t.com"}}),
    (3,  "vless-tcp-tls",            "vless",       "tcp",  "tls",      {"initial_client": {"email": "test3@t.com"}}),
    (4,  "vless-tcp-tls-vision",     "vless",       "tcp",  "tls",      {"initial_client": {"email": "test4@t.com"}}),
    (5,  "vless-ws-tls",             "vless",       "ws",   "tls",      {"ws_path":"/ws", "initial_client": {"email": "test5@t.com"}}),
    (6,  "vless-grpc-reality",       "vless",       "grpc", "reality",  {"grpc_service_name":"migate", "initial_client": {"email": "test6@t.com"}}),
    (7,  "vless-xhttp-reality",      "vless",       "xhttp","reality",  {"xhttp_path":"/","xhttp_mode":"stream-one", "initial_client": {"email": "test7@t.com"}}),
    (8,  "trojan-tcp-tls",           "trojan",      "tcp",  "tls",      {"initial_client": {"email": "test8@t.com"}}),
    (9,  "trojan-tcp-reality",       "trojan",      "tcp",  "reality",  {"initial_client": {"email": "test9@t.com"}}),
    (10, "trojan-grpc-reality",      "trojan",      "grpc", "reality",  {"grpc_service_name":"migate", "initial_client": {"email": "test10@t.com"}}),
    (11, "vmess-tcp-none",           "vmess",       "tcp",  "none",     {"initial_client": {"email": "test11@t.com"}}),
    (12, "vmess-tcp-tls",            "vmess",       "tcp",  "tls",      {"initial_client": {"email": "test12@t.com"}}),
    (13, "vmess-ws-none",            "vmess",       "ws",   "none",     {"ws_path":"/vmess", "initial_client": {"email": "test13@t.com"}}),
    (14, "vmess-ws-tls",             "vmess",       "ws",   "tls",      {"ws_path":"/vmess-ws", "initial_client": {"email": "test14@t.com"}}),
    (15, "ss-aes-256-gcm",           "shadowsocks", "tcp",  "none",     {"ss_method":"aes-256-gcm", "initial_client": {"email": "test15@t.com"}}),
    (16, "ss-2022-blake3",           "shadowsocks", "tcp",  "none",     {"ss_method":"2022-blake3-aes-256-gcm", "initial_client": {"email": "test16@t.com"}}),
    (17, "hysteria2",                None,          None,   None,       {}),
]

def ssh(cmd):
    r = subprocess.run(SSH_CMD + [cmd], capture_output=True, text=True, timeout=15)
    return r.stdout.strip(), r.returncode

def login():
    cj = http.cookiejar.CookieJar()
    op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    op.open(urllib.request.Request(
        f"{BASE}/api/login",
        data=json.dumps({"username":"admin","password":"admin"}).encode(),
        headers={"Content-Type":"application/json"}
    ))
    return op

def api_get(op, path):
    return json.loads(op.open(urllib.request.Request(f"{BASE}{path}")).read())

def api_post(op, path, data):
    resp = op.open(urllib.request.Request(
        f"{BASE}{path}",
        data=json.dumps(data).encode(),
        headers={"Content-Type":"application/json"}
    ))
    return resp.status, json.loads(resp.read())

def api_delete(op, path):
    op.open(urllib.request.Request(f"{BASE}{path}", method="DELETE"))

def check_config_has_inbound(config, port, proto, net, sec):
    """Check config structure, return (found, issues)"""
    for ib in config.get("inbounds", []):
        if ib.get("port") != port:
            continue
        ss = ib.get("streamSettings", {})
        issues = []
        if ib.get("protocol") != proto:
            issues.append(f"protocol={ib['protocol']}")
        if ss.get("network") != net:
            issues.append(f"network={ss.get('network')}")
        if ss.get("security") != sec:
            issues.append(f"security={ss.get('security')}")

        # Check transport-specific settings
        if net == "ws" and "wsSettings" not in ss:
            issues.append("missing wsSettings")
        if net == "grpc" and "grpcSettings" not in ss:
            issues.append("missing grpcSettings")
        if net == "xhttp" and "xhttpSettings" not in ss:
            issues.append("missing xhttpSettings")

        # Check security-specific settings
        if sec == "reality":
            if "realitySettings" not in ss:
                issues.append("missing realitySettings")
            else:
                rs = ss["realitySettings"]
                if not rs.get("privateKey"):
                    issues.append("missing privateKey in realitySettings")
                if not rs.get("serverNames"):
                    issues.append("missing serverNames in realitySettings")

        # Check flow for VLESS+REALITY
        if proto == "vless" and sec == "reality":
            clients = ib.get("settings", {}).get("clients", [])
            if clients and clients[0].get("flow") != "xtls-rprx-vision":
                issues.append(f"flow={clients[0].get('flow','')} != xtls-rprx-vision")
            elif not clients:
                issues.append("no clients in config")

        # Check protocol settings
        if proto == "trojan":
            clients = ib.get("settings", {}).get("clients", [])
            if clients and "password" not in clients[0]:
                issues.append("missing password in Trojan client")

        return True, issues or ["OK"]
    return False, [f"port {port} not found"]

def main():
    results = []
    start_port = 42000

    print("=" * 72)
    print("MiGate 协议兼容性测试 v3")
    print(f"VPS: {VPS_HOST}:{VPS_PORT}")
    print("=" * 72)

    # Health check
    out, rc = ssh("curl -s http://localhost:9999/api/health")
    if rc == 0 and out:
        print(f"  ✅ 服务正常: {out}")
    else:
        print(f"  ❌ 服务异常"); return

    # Login
    op = login()
    print("  ✅ 登录成功")

    # Cleanup old test inbounds (ports 40001-40016, 42001-42016)
    existing = api_get(op, "/api/inbounds").get("inbounds", [])
    test_ports = list(range(40001, 40017)) + list(range(42001, 42017))
    for ib in existing:
        if ib.get("port") in test_ports:
            try:
                api_delete(op, f"/api/inbounds/{ib['id']}")
                print(f"  清理: id={ib['id']} port={ib['port']} {ib.get('remark','')}")
            except:
                pass
    time.sleep(1)

    # Run tests
    for pid, remark, protocol, network, security, extra in PROTOCOLS:
        if protocol is None:
            print(f"\n{pid:2d}. {remark:30s} — MiGate 不支持 Hysteria2")
            results.append((pid, remark, "❌ 不支持", "MiGate 仅支持 VLESS/VMess/Trojan/Shadowsocks"))
            continue

        port = start_port + pid

        # Build payload
        payload = {
            "remark": remark, "protocol": protocol, "port": port,
            "network": network, "security": security,
        }
        payload.update(extra)

        notes_map = {
            1:"标准VLESS+REALITY", 2:"REALITY+flow", 3:"VLESS+TLS(无证书)",
            4:"TLS+XTLS-Vision", 5:"WSS", 6:"gRPC+REALITY", 7:"XHTTP+REALITY",
            8:"Trojan+TLS", 9:"Trojan+REALITY", 10:"Trojan+gRPC+REALITY",
            11:"VMess+TCP", 12:"VMess+TLS", 13:"VMess+WS",
            14:"VMess+WS+TLS", 15:"SS+AEAD", 16:"SS+SS2022",
        }
        notes = notes_map.get(pid, "")

        print(f"\n{'─'*60}")
        print(f"#{pid:2d} {remark:30s} port={port}  ({notes})")
        print(f"{'─'*60}")

        # Create inbound
        try:
            status, body = api_post(op, "/api/inbounds", payload)
            print(f"  ✅ 创建成功 → id={body.get('id')}, status={status}")
        except urllib.error.HTTPError as e:
            err_body = e.read().decode()
            print(f"  ❌ API 创建失败: HTTP {e.code} {err_body[:80]}")
            results.append((pid, remark, "❌ 创建失败", f"HTTP {e.code}: {err_body[:50]}"))
            continue
        except Exception as e:
            print(f"  ❌ 异常: {e}")
            results.append((pid, remark, "❌ 异常", str(e)[:50]))
            continue

        # Auto-apply happens in backend. Wait for Xray to restart
        time.sleep(3)

        # Check Xray service status
        out, rc = ssh("systemctl is-active xray 2>/dev/null")
        xray_ok = (rc == 0 and out == "active")
        print(f"  {'✅' if xray_ok else '⚠️'} Xray: {out}")

        # Fetch and verify Xray config
        config = api_get(op, "/api/xray/config")
        found, issues = check_config_has_inbound(config, port, protocol, network, security)
        if found:
            details = " | ".join(issues)
            print(f"  ✅ 配置验证: {details}")
        else:
            details = " | ".join(issues)
            print(f"  ❌ 配置验证失败: {details}")
            results.append((pid, remark, "❌ 配置异常", details[:55]))
            continue

        # Check port listening
        out, rc = ssh(f"ss -tlnp 2>/dev/null | grep -E ':{port} '")
        port_ok = rc == 0 and str(port) in out
        print(f"  {'✅' if port_ok else '⚠️'} 端口 {port}: {'监听中' if port_ok else '未监听'}")

        # Final result
        if port_ok:
            status_icon = "✅ 通过"
        elif xray_ok:
            status_icon = "⚠️ 配置正确但端口未监听"
        else:
            status_icon = "❌ Xray 异常"
        results.append((pid, remark, status_icon, details[:55]))

    # Summary
    print("\n\n" + "=" * 72)
    print("测试结果汇总")
    print("=" * 72)
    passed = sum(1 for *_, s, _ in results if s == "✅ 通过")
    failed = sum(1 for *_, s, _ in results if s.startswith("❌"))
    warned = sum(1 for *_, s, _ in results if s.startswith("⚠️") or s.startswith("❌ 不支持"))
    print(f"总计: {len(results)} | ✅ 通过: {passed} | ❌ 失败: {failed} | ⚠️ 警告/不支持: {warned}")
    print()
    for pid, remark, status, detail in results:
        icon = status[:2]
        print(f"  {pid:2d} {icon} {remark:<30s} {status:<16s} {detail[:50]}")

if __name__ == "__main__":
    main()
