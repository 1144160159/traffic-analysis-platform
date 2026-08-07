export function compactTopicTopologyLabel(value: string, maxCharacters = 18) {
  const normalized = String(value ?? '').trim();
  const characters = Array.from(normalized);
  if (characters.length <= maxCharacters) return normalized;

  const available = Math.max(4, maxCharacters - 1);
  const headLength = Math.ceil(available * 0.6);
  const tailLength = available - headLength;
  return `${characters.slice(0, headLength).join('')}…${characters.slice(-tailLength).join('')}`;
}

export function safeTopicTopologyRichText(value: string) {
  return value.replace(/\{/gu, '｛').replace(/\}/gu, '｝').replace(/\|/gu, '｜');
}
