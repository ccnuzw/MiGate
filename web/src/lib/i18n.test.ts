import { describe, expect, it } from 'vitest';
import { translateElement, translateText } from './i18n';

describe('i18n text translation', () => {
  it('keeps Chinese text in Chinese mode', () => {
    expect(translateText('入站与客户端', 'zh')).toBe('入站与客户端');
  });

  it('translates major page copy and dynamic messages in English mode', () => {
    const samples = [
      ['入站与客户端', 'Inbounds and clients'],
      ['导入 SOCKS5 地址池', 'Import SOCKS5 pool'],
      ['路由规则已保存', 'Routing rule saved'],
      ['应用 Xray 配置？', 'Apply Xray config?'],
      ['设置已保存，端口或路径变更需要重启服务后生效', 'Settings saved. Port or path changes require a service restart.'],
      ['Web 基础路径', 'Web base path'],
      ['TLS 证书文件', 'TLS cert file'],
      ['HY2 混淆密码', 'HY2 obfs password'],
      ['按来源入站、域名、IP、规则集或协议选择出站链路。', 'Choose outbound links by inbound source, domain, IP, rule set, or protocol.'],
      ['支持 geoip:cn、CIDR、单 IP，逗号或换行分隔。', 'Supports geoip:cn, CIDR, or single IP values separated by commas or newlines.'],
      ['登录失败，请检查用户名或密码', 'Login failed. Check username or password.'],
    ] as const;

    for (const [source, expected] of samples) {
      expect(translateText(source, 'en')).toBe(expected);
    }
  });

  it('keeps separate original text for multiple text nodes under one parent', () => {
    const host = document.createElement('div');
    host.append('入站');
    host.append(document.createElement('span'));
    host.append('出站');

    translateElement(host, 'en');
    expect(host.textContent).toBe('InboundsOutbounds');

    translateElement(host, 'zh');
    expect(host.textContent).toBe('入站出站');
  });

  it('does not restore stale Chinese over React-rendered English on language switches', () => {
    const host = document.createElement('div');
    host.append('1 活跃 · 0 过期 · 0 受限');

    translateElement(host, 'en');
    expect(host.textContent).toBe('1 Active · 0 Expiry · 0 Limited');

    host.textContent = '1 Active · 0 Expiry · 0 Limited';
    translateElement(host, 'en');
    expect(host.textContent).toBe('1 Active · 0 Expiry · 0 Limited');
  });
});
