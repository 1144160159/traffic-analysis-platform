import {
  AuditOutlined,
  BankOutlined,
  CheckCircleOutlined,
  DownOutlined,
  LoginOutlined,
  LockOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  UserSwitchOutlined,
  UserOutlined,
} from '@ant-design/icons';
import type { ComponentType, HTMLAttributes } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Checkbox, Form, Input, Select, message } from 'antd';
import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import loginHologramShieldScreenshotIcon from '@/assets/screenshot-icons/login-hologram-shield-alpha.png';
import loginShieldScreenshotIcon from '@/assets/screenshot-icons/login-shield-screenshot.png';
import { appConfig } from '@/config/runtime';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { buildOidcLoginUrl, fetchCaptcha, login, localBypassUser } from '@/services/api';
import type { CaptchaChallenge } from '@/services/api';
import { setAuthTokens } from '@/services/authStorage';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type LoginForm = {
  tenantId?: string;
  username: string;
  password: string;
  captchaCode?: string;
};

type LoginMethod = 'password' | 'sso';
type LoginCapabilityIconKind = 'encrypted' | 'identity' | 'audit';

const loginOverlays: OverlayContract[] = [
  {
    id: 'modal-login-error-captcha',
    title: '登录异常与验证码状态',
    kind: 'Modal',
    actionLabel: '登录异常',
    description: '展示验证码加载失败、账号锁定、OIDC 异常和登录失败重试提示。',
    impact: '仅展示认证错误上下文，不暴露密码、令牌或后端敏感响应。',
    audit: '记录登录失败原因分类、租户、账号和客户端 trace。',
  },
];

const loginCapabilityIconSources = {
  encrypted: 'https://raw.githubusercontent.com/ant-design/ant-design-icons/master/packages/icons-svg/svg/outlined/safety-certificate.svg',
  identity: 'https://raw.githubusercontent.com/ant-design/ant-design-icons/master/packages/icons-svg/svg/outlined/user-switch.svg',
  audit: 'https://raw.githubusercontent.com/ant-design/ant-design-icons/master/packages/icons-svg/svg/outlined/audit.svg',
} satisfies Record<LoginCapabilityIconKind, string>;

const loginCapabilityIcons = {
  encrypted: SafetyCertificateOutlined,
  identity: UserSwitchOutlined,
  audit: AuditOutlined,
} satisfies Record<LoginCapabilityIconKind, ComponentType<HTMLAttributes<HTMLSpanElement>>>;

const DEFAULT_LOGIN_TENANT_ID = 'default';
const loginSiteOptions = [
  { value: DEFAULT_LOGIN_TENANT_ID, label: '主园区' },
  { value: 'teaching-campus', label: '教学区' },
  { value: 'datacenter-campus', label: '数据中心' },
];

function CapabilityIcon({ kind }: { kind: LoginCapabilityIconKind }) {
  const Icon = loginCapabilityIcons[kind];
  return (
    <Icon
      className="taf-login__capability-svg-icon"
      aria-hidden="true"
      data-icon-source="Ant Design Icons"
      data-icon-source-url={loginCapabilityIconSources[kind]}
      data-icon-name={kind}
    />
  );
}

export function LoginPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [form] = Form.useForm<LoginForm>();
  const [captcha, setCaptcha] = useState<CaptchaChallenge>();
  const [captchaError, setCaptchaError] = useState('');
  const [loginMethod, setLoginMethod] = useState<LoginMethod>('password');
  const [submitting, setSubmitting] = useState(false);
  const [ssoSubmitting, setSsoSubmitting] = useState(false);
  const visualShieldMode =
    typeof window !== 'undefined' &&
    (new URLSearchParams(window.location.search).get('__taf_visual') === '1' || isVisualBreakdownMode());
  const requiresCaptcha = appConfig.authEnabled && !appConfig.useMock;

  useEffect(() => {
    const root = document.getElementById('root');

    document.documentElement.classList.add('taf-login-document');
    document.body.classList.add('taf-login-document');
    root?.classList.add('taf-login-root');

    return () => {
      document.documentElement.classList.remove('taf-login-document');
      document.body.classList.remove('taf-login-document');
      root?.classList.remove('taf-login-root');
    };
  }, []);

  const loadCaptcha = useCallback(async () => {
    if (!requiresCaptcha) return;
    try {
      setCaptchaError('');
      const challenge = await fetchCaptcha();
      setCaptcha(challenge);
      form.setFieldValue('captchaCode', '');
    } catch (error) {
      setCaptcha(undefined);
      setCaptchaError(error instanceof Error ? error.message : '验证码加载失败');
    }
  }, [form, requiresCaptcha]);

  useEffect(() => {
    void loadCaptcha();
  }, [loadCaptcha]);

  const onFinish = async (values: LoginForm) => {
    try {
      setSubmitting(true);
      const result = await login({
        tenant_id: values.tenantId || DEFAULT_LOGIN_TENANT_ID,
        username: values.username,
        password: values.password,
        captcha_id: captcha?.captchaId,
        captcha_code: values.captchaCode,
      });
      setAuthTokens(result.token, result.refreshToken);
      queryClient.setQueryData(['current-user'], result.user);
      message.success(`欢迎回来，${result.username}`);
      navigate('/dashboard', { replace: true });
    } catch (error) {
      message.error(error instanceof Error ? error.message : '登录失败');
      void loadCaptcha();
    } finally {
      setSubmitting(false);
    }
  };

  const startSsoLogin = () => {
    const tenantId = form.getFieldValue('tenantId') || DEFAULT_LOGIN_TENANT_ID;
    if (!appConfig.authEnabled || appConfig.useMock) {
      const user = { ...localBypassUser, username: 'oidc_user', tenantId };
      setAuthTokens('mock-oidc-token');
      queryClient.setQueryData(['current-user'], user);
      message.success(`欢迎回来，${user.username}`);
      navigate('/dashboard', { replace: true });
      return;
    }

    const callbackUrl = new URL('/oidc/callback', window.location.origin);
    callbackUrl.searchParams.set('next', '/dashboard');
    setSsoSubmitting(true);
    window.location.assign(buildOidcLoginUrl({ tenantId, redirectUrl: callbackUrl.toString() }));
  };

  return (
    <main className={`taf-login${visualShieldMode ? ' taf-login--visual-target' : ''}`}>
      <div className="taf-login__visual" aria-hidden="true">
        <span className="taf-login__campus" />
        <span className="taf-login__data-rain" />
        <span className="taf-login__bottom-veil" />
      </div>
      <section className="taf-login__hero" aria-label="系统身份认证介绍">
        <div className="taf-login__hologram">
          <img
            className="taf-login__hologram-screenshot"
            src={loginHologramShieldScreenshotIcon}
            alt=""
            aria-hidden="true"
            data-generated-icon="screenshot-login-hologram-shield"
            decoding="async"
            draggable={false}
          />
          <span className="taf-login__shield-core" aria-hidden="true">
            <img
              className="taf-login__shield-icon taf-login__shield-icon--screenshot"
              src={loginShieldScreenshotIcon}
              alt=""
              aria-hidden="true"
              data-generated-icon="screenshot-login-shield"
              decoding="async"
              draggable={false}
            />
          </span>
        </div>
        <div className="taf-login__hero-title">
          <h1>{appConfig.productName}</h1>
          <p><span />统一身份认证入口<span /></p>
        </div>
        <div className="taf-login__capabilities">
          <span className="taf-login__capability">
            <span className="taf-login__capability-icon">
              <CapabilityIcon kind="encrypted" />
            </span>
            <span className="taf-login__capability-label">加密传输</span>
          </span>
          <span className="taf-login__capability">
            <span className="taf-login__capability-icon">
              <CapabilityIcon kind="identity" />
            </span>
            <span className="taf-login__capability-label">身份校验</span>
          </span>
          <span className="taf-login__capability">
            <span className="taf-login__capability-icon">
              <CapabilityIcon kind="audit" />
            </span>
            <span className="taf-login__capability-label">审计留痕</span>
          </span>
        </div>
        <ul className="taf-login__assurance">
          <li><CheckCircleOutlined />请使用授权账号访问</li>
          <li><CheckCircleOutlined />多因素校验已启用</li>
          <li><span className="taf-login__info-dot">i</span>登录失败请联系管理员</li>
        </ul>
      </section>
      <section className="taf-login__panel">
        <div className="taf-login__tabs" role="tablist" aria-label="登录方式">
          <button
            className={loginMethod === 'password' ? 'is-active' : undefined}
            type="button"
            role="tab"
            aria-selected={loginMethod === 'password'}
            aria-controls="taf-login-password-panel"
            onClick={() => setLoginMethod('password')}
          >
            账号密码登录
          </button>
          <button
            className={loginMethod === 'sso' ? 'is-active' : undefined}
            type="button"
            role="tab"
            aria-selected={loginMethod === 'sso'}
            aria-controls="taf-login-sso-panel"
            onClick={() => setLoginMethod('sso')}
          >
            OIDC / SSO
          </button>
        </div>
        <OverlayContractHost overlays={loginOverlays} compact />
        <Form<LoginForm>
          form={form}
          layout="vertical"
          initialValues={{ tenantId: DEFAULT_LOGIN_TENANT_ID }}
          onFinish={onFinish}
        >
          <Form.Item label="租户 / 站点">
            <div className="taf-login__site-select">
              <BankOutlined aria-hidden="true" />
              <Form.Item name="tenantId" noStyle>
                <Select
                  suffixIcon={<DownOutlined />}
                  placeholder="请选择租户 / 站点"
                  options={loginSiteOptions}
                  aria-label="租户 / 站点"
                />
              </Form.Item>
            </div>
          </Form.Item>
          {loginMethod === 'password' ? (
            <div id="taf-login-password-panel" role="tabpanel" aria-label="账号密码登录">
              <Form.Item name="username" label="账号" rules={[{ required: true, message: '请输入账号' }]}>
                <Input prefix={<UserOutlined />} placeholder="请输入账号" autoComplete="username" />
              </Form.Item>
              <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
                <Input.Password prefix={<LockOutlined />} placeholder="请输入密码" autoComplete="current-password" />
              </Form.Item>
              {requiresCaptcha && (
                <div className="taf-login__captcha-row">
                  <Form.Item name="captchaCode" label="验证码" rules={[{ required: true, message: '请输入验证码' }]}>
                    <Input prefix={<SafetyOutlined />} placeholder="请输入验证码" autoComplete="off" />
                  </Form.Item>
                  <button className="taf-login__captcha" type="button" onClick={loadCaptcha}>
                    {captcha?.imageData ? <img src={captcha.imageData} alt="登录验证码" /> : <span>{captchaError || '加载中'}</span>}
                  </button>
                  <button className="taf-login__captcha-refresh" type="button" aria-label="刷新验证码" onClick={loadCaptcha}>
                    <ReloadOutlined />
                  </button>
                </div>
              )}
              <div className="taf-login__options">
                <Checkbox defaultChecked>记住登录</Checkbox>
                <nav aria-label="登录帮助">
                  <a>忘记密码</a>
                  <a>帮助中心</a>
                  <a>隐私声明</a>
                </nav>
              </div>
              <Button className="taf-login__submit" type="primary" htmlType="submit" loading={submitting} block>
                登录
              </Button>
            </div>
          ) : (
            <div className="taf-login__sso-panel" id="taf-login-sso-panel" role="tabpanel" aria-label="OIDC / SSO 登录">
              <div className="taf-login__sso-card">
                <span className="taf-login__sso-icon"><LoginOutlined /></span>
                <strong>统一身份提供方</strong>
                <p>使用已接入的园区身份中心完成 OIDC 授权码登录，回调后自动进入运营工作台。</p>
              </div>
              <Button
                className="taf-login__submit"
                type="primary"
                loading={ssoSubmitting}
                onClick={startSsoLogin}
                block
              >
                通过 OIDC / SSO 登录
              </Button>
            </div>
          )}
        </Form>
      </section>
    </main>
  );
}
