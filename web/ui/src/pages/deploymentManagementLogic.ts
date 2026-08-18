export function deploymentStatusLabel(status: string, percentage = 0) {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'planned') return '待发布';
  if (normalized === 'gray') return `灰度中 ${percentage || 20}%`;
  if (normalized === 'active') return '已发布';
  if (normalized === 'paused') return '已暂停';
  if (normalized === 'rolled_back') return '已回滚';
  if (normalized === 'failed') return '阻断';
  if (normalized === 'cancelled') return '已取消';
  if (normalized === 'superseded') return '已替代';
  return status || '未知';
}

export function hasDeployScope(permissions: string[], required: string) {
  return permissions.some((permission) => permission === '*' || permission === 'admin:*' || permission === 'deploy:*' || permission === required);
}

export function deploymentActionAvailability(status: string, permissions: string[]) {
  const canCreate = hasDeployScope(permissions, 'deploy:create');
  const canGray = hasDeployScope(permissions, 'deploy:gray');
  const canActivate = hasDeployScope(permissions, 'deploy:activate');
  const canRollbackPermission = hasDeployScope(permissions, 'deploy:rollback');
  return {
    canCreate,
    canGray,
    canActivate,
    canContinue: ['planned', 'gray', 'paused'].includes(status) && (status === 'planned' ? canGray : canActivate),
    canEditScope: status === 'planned' && canGray,
    canPause: ['gray', 'active'].includes(status) && canActivate,
    canRollback: ['gray', 'active', 'paused', 'failed'].includes(status) && canRollbackPermission,
  };
}

export function deploymentRuntimeExpansionBlocked(
  status: string,
  gate?: { enabled: boolean; expansion_allowed: boolean },
) {
  return status === 'gray' && Boolean(gate?.enabled) && !gate?.expansion_allowed;
}
