use anyhow::{Context, Result};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::RwLock;
use tonic::metadata::MetadataValue;
use tonic::Request;
use tracing::{debug, info, warn};

#[derive(Clone, Debug)]
pub enum TokenRefreshStrategy {
    Static,
    Periodic(Duration),
    BeforeExpiry(Duration),
}

#[derive(Clone, Debug)]
pub struct AuthConfig {
    pub token: String,
    pub refresh_strategy: TokenRefreshStrategy,
    pub ttl: Option<Duration>,
}

impl AuthConfig {
    pub fn static_token(token: String) -> Self {
        Self {
            token,
            refresh_strategy: TokenRefreshStrategy::Static,
            ttl: None,
        }
    }

    pub fn refreshable_token(token: String, ttl: Duration, refresh_before: Duration) -> Self {
        Self {
            token,
            refresh_strategy: TokenRefreshStrategy::BeforeExpiry(refresh_before),
            ttl: Some(ttl),
        }
    }
}

#[derive(Clone)]
pub struct AuthProvider {
    token: Arc<RwLock<String>>,
    expires_at: Arc<RwLock<Option<Instant>>>,
    refresh_strategy: TokenRefreshStrategy,
    refresh_fn: Option<Arc<dyn Fn() -> Result<String> + Send + Sync>>,
    enabled: bool,
}

impl AuthProvider {
    pub fn from_config(config: AuthConfig) -> Self {
        let expires_at = config.ttl.map(|ttl| Instant::now() + ttl);

        info!(
            "Auth provider created: strategy={:?}, ttl={:?}",
            config.refresh_strategy, config.ttl
        );

        Self {
            token: Arc::new(RwLock::new(config.token)),
            expires_at: Arc::new(RwLock::new(expires_at)),
            refresh_strategy: config.refresh_strategy,
            refresh_fn: None,
            enabled: true,
        }
    }

    pub fn static_token(token: String) -> Self {
        Self::from_config(AuthConfig::static_token(token))
    }

    pub fn dynamic_token<F>(initial_token: String, ttl: Duration, refresh_fn: F) -> Self
    where
        F: Fn() -> Result<String> + Send + Sync + 'static,
    {
        let expires_at = Instant::now() + ttl;

        Self {
            token: Arc::new(RwLock::new(initial_token)),
            expires_at: Arc::new(RwLock::new(Some(expires_at))),
            refresh_strategy: TokenRefreshStrategy::BeforeExpiry(Duration::from_secs(300)),
            refresh_fn: Some(Arc::new(refresh_fn)),
            enabled: true,
        }
    }

    pub fn disabled() -> Self {
        Self {
            token: Arc::new(RwLock::new(String::new())),
            expires_at: Arc::new(RwLock::new(None)),
            refresh_strategy: TokenRefreshStrategy::Static,
            refresh_fn: None,
            enabled: false,
        }
    }

    pub fn is_enabled(&self) -> bool {
        self.enabled
    }

    pub async fn get_token(&self) -> Result<String> {
        if !self.enabled {
            return Ok(String::new());
        }

        if self.should_refresh().await {
            if let Err(e) = self.refresh_token().await {
                warn!("Token refresh failed, using stale token: {}", e);
            }
        }

        let token = self.token.read().await.clone();

        if token.is_empty() && self.enabled {
            anyhow::bail!("Token is empty but authentication is enabled");
        }

        Ok(token)
    }

    async fn should_refresh(&self) -> bool {
        match self.refresh_strategy {
            TokenRefreshStrategy::Static => false,

            TokenRefreshStrategy::Periodic(interval) => {
                if let Some(expires_at) = *self.expires_at.read().await {
                    Instant::now() >= expires_at - interval
                } else {
                    false
                }
            }

            TokenRefreshStrategy::BeforeExpiry(before) => {
                if let Some(expires_at) = *self.expires_at.read().await {
                    Instant::now() >= expires_at - before
                } else {
                    false
                }
            }
        }
    }

    async fn refresh_token(&self) -> Result<()> {
        if let Some(ref refresh_fn) = self.refresh_fn {
            debug!("Refreshing authentication token");

            match refresh_fn() {
                Ok(new_token) => {
                    *self.token.write().await = new_token;

                    if let TokenRefreshStrategy::BeforeExpiry(_)
                    | TokenRefreshStrategy::Periodic(_) = self.refresh_strategy
                    {
                        *self.expires_at.write().await =
                            Some(Instant::now() + Duration::from_secs(3600));
                    }

                    info!("Token refreshed successfully");
                    Ok(())
                }
                Err(e) => {
                    warn!("Failed to refresh token: {}", e);
                    Err(e)
                }
            }
        } else {
            Ok(())
        }
    }

    pub async fn set_token(&self, token: String, ttl: Option<Duration>) {
        *self.token.write().await = token;

        if let Some(ttl) = ttl {
            *self.expires_at.write().await = Some(Instant::now() + ttl);
        }

        info!("Token updated manually");
    }

    pub async fn add_to_request<T>(&self, request: &mut Request<T>) -> Result<()> {
        if !self.enabled {
            return Ok(());
        }

        let token = self.get_token().await?;

        if token.is_empty() {
            return Ok(());
        }

        let auth_value = format!("Bearer {}", token)
            .parse::<MetadataValue<_>>()
            .context("Invalid token format")?;

        request.metadata_mut().insert("authorization", auth_value);

        debug!("Added authorization header to request");

        Ok(())
    }

    pub async fn add_to_request_with_headers<T>(
        &self,
        request: &mut Request<T>,
        use_tenant_token_header: bool,
        custom_headers: Option<&HashMap<&'static str, String>>,
    ) -> Result<()> {
        if !self.enabled {
            return Ok(());
        }

        let token = self.get_token().await?;

        debug!("🔍 Adding authentication to request:");
        debug!("  Token present: {}", !token.is_empty());
        debug!("  Use tenant-token header: {}", use_tenant_token_header);
        debug!(
            "  Custom headers count: {}",
            custom_headers.map(|h| h.len()).unwrap_or(0)
        );

        if !token.is_empty() {
            if use_tenant_token_header {
                let token_value = token
                    .parse::<MetadataValue<_>>()
                    .context("Invalid token format")?;

                request.metadata_mut().insert("x-tenant-token", token_value);
                debug!(
                    "  ✅ Added x-tenant-token: {}...",
                    &token[..10.min(token.len())]
                );
            } else {
                let auth_value = format!("Bearer {}", token)
                    .parse::<MetadataValue<_>>()
                    .context("Invalid token format")?;

                request.metadata_mut().insert("authorization", auth_value);
                debug!("  ✅ Added authorization header");
            }
        }

        if let Some(headers) = custom_headers {
            for (&key, value) in headers.iter() {
                if let Ok(header_value) = value.parse::<MetadataValue<_>>() {
                    request.metadata_mut().insert(key, header_value);
                    debug!("  ✅ Added custom header: {} = {}", key, value);
                } else {
                    warn!("  ⚠ Invalid header value for key: {}", key);
                }
            }
        }

        debug!("📋 Final request metadata:");
        for key_and_value in request.metadata().iter() {
            match key_and_value {
                tonic::metadata::KeyAndValueRef::Ascii(key, value) => {
                    debug!("  {} = {:?}", key, value);
                }
                tonic::metadata::KeyAndValueRef::Binary(key, value) => {
                    debug!("  {} = {:?} (binary)", key, value);
                }
            }
        }

        Ok(())
    }

    pub async fn token_info(&self) -> TokenInfo {
        let expires_at = *self.expires_at.read().await;

        let (is_expired, time_until_expiry) = if let Some(exp) = expires_at {
            let now = Instant::now();
            if now >= exp {
                (true, None)
            } else {
                (false, Some(exp - now))
            }
        } else {
            (false, None)
        };

        TokenInfo {
            enabled: self.enabled,
            is_expired,
            time_until_expiry,
            refresh_strategy: self.refresh_strategy.clone(),
        }
    }

    pub fn start_refresh_task(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(60));

            loop {
                interval.tick().await;

                if self.should_refresh().await {
                    if let Err(e) = self.refresh_token().await {
                        warn!("Background token refresh failed: {}", e);
                    }
                }
            }
        })
    }
}

#[derive(Debug, Clone)]
pub struct TokenInfo {
    pub enabled: bool,
    pub is_expired: bool,
    pub time_until_expiry: Option<Duration>,
    pub refresh_strategy: TokenRefreshStrategy,
}

impl std::fmt::Display for TokenInfo {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if !self.enabled {
            return write!(f, "Authentication disabled");
        }

        if self.is_expired {
            write!(f, "Token expired")
        } else if let Some(ttl) = self.time_until_expiry {
            write!(f, "Token valid for {} seconds", ttl.as_secs())
        } else {
            write!(f, "Token valid (no expiry)")
        }
    }
}
