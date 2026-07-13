import { CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { Button, Result, Spin, message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { fetchCurrentUser } from '@/services/api';
import { consumeOidcCallbackTokens } from '@/services/authStorage';

const safeNextPath = (value: string | null) => {
  if (!value || !value.startsWith('/') || value.startsWith('//')) return '/dashboard';
  if (value === '/login' || value.startsWith('/oidc/callback')) return '/dashboard';
  return value;
};

export function OidcCallbackPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [error, setError] = useState('');
  const nextPath = useMemo(() => safeNextPath(new URLSearchParams(window.location.search).get('next')), []);

  useEffect(() => {
    const finishLogin = async () => {
      const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''));
      const searchParams = new URLSearchParams(window.location.search);
      const providerError = hashParams.get('error') ?? searchParams.get('error');
      if (providerError) {
        setError(hashParams.get('error_description') ?? searchParams.get('error_description') ?? providerError);
        return;
      }

      if (!consumeOidcCallbackTokens()) {
        setError('OIDC 回调未返回访问令牌');
        return;
      }

      try {
        const user = await fetchCurrentUser();
        queryClient.setQueryData(['current-user'], user);
        message.success(`欢迎回来，${user.username}`);
        navigate(nextPath, { replace: true });
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'OIDC 登录态校验失败');
      }
    };

    void finishLogin();
  }, [navigate, nextPath, queryClient]);

  return (
    <main className="taf-login taf-oidc-callback">
      <div className="taf-login__visual" aria-hidden="true">
        <span className="taf-login__campus" />
        <span className="taf-login__data-rain" />
        <span className="taf-login__bottom-veil" />
      </div>
      <section className="taf-oidc-callback__panel" aria-live="polite">
        {error ? (
          <Result
            status="error"
            icon={<CloseCircleOutlined />}
            title="OIDC / SSO 登录失败"
            subTitle={error}
            extra={
              <Button type="primary" onClick={() => navigate('/login', { replace: true })}>
                返回登录
              </Button>
            }
          />
        ) : (
          <Result
            status="info"
            icon={<Spin indicator={<LoadingOutlined spin />} />}
            title="正在完成 OIDC / SSO 登录"
            subTitle="身份令牌校验中"
            extra={<CheckCircleOutlined className="taf-oidc-callback__check" />}
          />
        )}
      </section>
    </main>
  );
}
