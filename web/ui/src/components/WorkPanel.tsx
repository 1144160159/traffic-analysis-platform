export function WorkPanel({
  title,
  extra,
  children,
  className = '',
}: {
  title: string;
  extra?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={`taf-panel ${className}`}>
      <div className="taf-panel__header">
        <h2>{title}</h2>
        {extra && <div className="taf-panel__extra">{extra}</div>}
      </div>
      <div className="taf-panel__body">{children}</div>
    </section>
  );
}
