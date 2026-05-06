import { clsx } from "clsx";
import type { ReactNode } from "react";

interface CardProps {
  children: ReactNode;
  className?: string;
  title?: string;
  titleRight?: ReactNode;
}

export function Card({ children, className, title, titleRight }: CardProps) {
  return (
    <div
      className={clsx(
        "rounded-lg border border-surface-3 bg-surface-1 flex flex-col",
        className
      )}
    >
      {title && (
        <div className="flex items-center justify-between px-4 py-3 border-b border-surface-3">
          <span className="text-xs font-semibold uppercase tracking-widest text-text-secondary">
            {title}
          </span>
          {titleRight && <div className="flex items-center gap-2">{titleRight}</div>}
        </div>
      )}
      <div className="flex-1 min-h-0">{children}</div>
    </div>
  );
}

interface BadgeProps {
  variant: "success" | "warning" | "danger" | "critical" | "info" | "muted";
  children: ReactNode;
  size?: "sm" | "xs";
}

const badgeVariants = {
  success: "bg-success/10 text-success border-success/30",
  warning: "bg-warning/10 text-warning border-warning/30",
  danger: "bg-danger/10 text-danger border-danger/30",
  critical: "bg-critical/10 text-critical border-critical/30",
  info: "bg-brand/10 text-brand border-brand/30",
  muted: "bg-surface-3 text-text-secondary border-surface-4",
};

export function Badge({ variant, children, size = "sm" }: BadgeProps) {
  return (
    <span
      className={clsx(
        "inline-flex items-center border rounded font-mono font-semibold",
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-1.5 py-px text-[10px]",
        badgeVariants[variant]
      )}
    >
      {children}
    </span>
  );
}

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "danger" | "ghost" | "warning";
  size?: "sm" | "md";
  loading?: boolean;
}

const buttonVariants = {
  primary: "bg-brand hover:bg-brand-dim text-white border-transparent",
  secondary: "bg-surface-3 hover:bg-surface-4 text-text-primary border-surface-4",
  danger: "bg-danger/10 hover:bg-danger/20 text-danger border-danger/30",
  ghost: "bg-transparent hover:bg-surface-2 text-text-secondary border-transparent",
  warning: "bg-warning/10 hover:bg-warning/20 text-warning border-warning/30",
};

export function Button({ variant = "secondary", size = "sm", loading, children, className, disabled, ...props }: ButtonProps) {
  return (
    <button
      disabled={disabled || loading}
      className={clsx(
        "inline-flex items-center gap-1.5 border rounded font-medium transition-colors duration-100",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        size === "sm" ? "px-3 py-1.5 text-xs" : "px-4 py-2 text-sm",
        buttonVariants[variant],
        className
      )}
      {...props}
    >
      {loading && <Spinner size="xs" />}
      {children}
    </button>
  );
}

interface SpinnerProps {
  size?: "xs" | "sm" | "md";
}

const spinnerSizes = { xs: "w-3 h-3", sm: "w-4 h-4", md: "w-5 h-5" };

export function Spinner({ size = "sm" }: SpinnerProps) {
  return (
    <svg
      className={clsx("animate-spin text-current", spinnerSizes[size])}
      viewBox="0 0 24 24"
      fill="none"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
      />
    </svg>
  );
}

interface StatRowProps {
  label: string;
  value: ReactNode;
  mono?: boolean;
}

export function StatRow({ label, value, mono }: StatRowProps) {
  return (
    <div className="flex items-center justify-between py-1.5 border-b border-surface-2 last:border-0">
      <span className="text-xs text-text-tertiary">{label}</span>
      <span className={clsx("text-xs text-text-primary", mono && "font-mono")}>{value}</span>
    </div>
  );
}

interface ProgressBarProps {
  value: number;
  max?: number;
  variant?: "success" | "warning" | "danger";
  className?: string;
}

export function ProgressBar({ value, max = 1, variant, className }: ProgressBarProps) {
  const pct = Math.min(100, (value / max) * 100);
  const auto = pct > 80 ? "danger" : pct > 60 ? "warning" : "success";
  const color = variant ?? auto;
  const colors = { success: "bg-success", warning: "bg-warning", danger: "bg-danger" };
  return (
    <div className={clsx("h-1.5 w-full bg-surface-3 rounded-full overflow-hidden", className)}>
      <div
        className={clsx("h-full rounded-full transition-all duration-300", colors[color])}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

export function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-center gap-2 px-3 py-2 rounded border border-danger/30 bg-danger/5 text-danger text-xs">
      <span className="shrink-0">⚠</span>
      <span>{message}</span>
    </div>
  );
}

export function EmptyState({ message }: { message: string }) {
  return (
    <div className="flex items-center justify-center h-full min-h-[80px] text-xs text-text-tertiary">
      {message}
    </div>
  );
}

export function Dot({ className }: { className?: string }) {
  return <span className={clsx("inline-block w-2 h-2 rounded-full", className)} />;
}
