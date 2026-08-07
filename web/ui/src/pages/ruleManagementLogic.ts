export function ruleLifecycleLabel(status: string): '草稿' | '待审' | '灰度' | '启用' | '停用' | '回滚' {
  const normalized = status.trim().toLowerCase();
  if (['rollback', '回滚'].some((value) => normalized.includes(value))) return '回滚';
  if (['disabled', 'inactive', 'deprecated', 'archived', '停用', '禁用'].some((value) => normalized.includes(value))) return '停用';
  if (['gray', 'canary', '灰度'].some((value) => normalized.includes(value))) return '灰度';
  if (['pending', 'review', '待审'].some((value) => normalized.includes(value))) return '待审';
  if (['active', 'enabled', '启用'].some((value) => normalized.includes(value))) return '启用';
  return '草稿';
}
