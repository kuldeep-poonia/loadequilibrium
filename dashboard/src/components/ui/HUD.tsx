'use client';

import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { type HTMLMotionProps, motion } from 'framer-motion';
import { useEffect, useState } from 'react';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * TacticalBox — primary panel frame, Apple-clean neumorphic
 */
export function TacticalBox({
  children,
  className,
  title,
  badge,
  status = 'nominal',
  scan = false,
  ...props
}: {
  children: React.ReactNode;
  className?: string;
  title?: string;
  badge?: string;
  status?: 'nominal' | 'warning' | 'critical' | 'alert';
  scan?: boolean;
} & HTMLMotionProps<'div'>) {
  const accentColor =
    status === 'critical' || status === 'alert'
      ? '#FF453A'
      : status === 'warning'
      ? '#FF9F0A'
      : '#00D4FF';

  const chipClass =
    status === 'critical' || status === 'alert'
      ? 'chip chip--red'
      : status === 'warning'
      ? 'chip chip--amber'
      : 'chip chip--cyan';

  return (
    <motion.div
      className={cn(
        'neo-panel relative flex min-h-0 min-w-0 flex-col overflow-hidden rounded-2xl',
        className
      )}
      {...props}
    >
      {title && (
        <div className="flex min-w-0 items-center justify-between gap-3 border-b border-white/[0.05] px-4 py-2.5">
          <div className="flex min-w-0 items-center gap-2.5">
            <div
              className="h-[14px] w-[2px] flex-shrink-0 rounded-full opacity-80"
              style={{ background: accentColor }}
            />
            <span className="label-xs truncate" style={{ color: '#A8ABB4', letterSpacing: '0.16em' }}>
              {title}
            </span>
          </div>
          {badge && (
            <span className={cn(chipClass, 'flex-shrink-0')}>{badge}</span>
          )}
        </div>
      )}
      {scan && (
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-x-0 top-0 h-px"
          style={{ background: 'rgba(0,212,255,0.35)' }}
        />
      )}
      <div className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden p-3.5 xl:p-4">
        {children}
      </div>
    </motion.div>
  );
}

/**
 * StatusTube — clean neumorphic gauge
 */
export function StatusTube({
  label,
  value,
  percent,
  vertical = false,
  status = 'nominal',
}: {
  label: string;
  value: string | number;
  percent: number;
  vertical?: boolean;
  status?: 'nominal' | 'warning' | 'critical';
}) {
  const safePercent = Math.min(100, Math.max(0, percent * 100));
  const barColor =
    status === 'critical' ? '#FF453A' : status === 'warning' ? '#FF9F0A' : '#00D4FF';

  return (
    <div className={cn('flex gap-3', vertical ? 'flex-col items-center' : 'flex-col')}>
      <div className="flex items-end justify-between px-0.5">
        <div className="flex flex-col">
          <span className="label-xs">{label}</span>
          <span className="data-value mt-1 text-sm font-semibold" style={{ color: barColor }}>
            {value}
          </span>
        </div>
        <span className="label-xs opacity-50">{Math.round(safePercent)}%</span>
      </div>
      <div className={cn('progress-track', vertical ? 'h-28 w-[3px]' : 'w-full')}>
        <div
          className="progress-bar"
          style={{
            [vertical ? 'height' : 'width']: `${safePercent}%`,
            background: barColor,
          }}
        />
      </div>
    </div>
  );
}

/**
 * DigitalLog — clean monospace console
 */
export function DigitalLog({ lines }: { lines: string[] }) {
  const [cursor, setCursor] = useState(true);
  useEffect(() => {
    const itv = setInterval(() => setCursor((p) => !p), 600);
    return () => clearInterval(itv);
  }, []);

  return (
    <div
      className="h-full space-y-1 overflow-y-auto pr-1"
      style={{ fontFamily: 'var(--font-data)', fontSize: '10px', lineHeight: '1.6' }}
    >
      {lines.map((line, i) => (
        <div key={i} className="flex gap-3 group">
          <span style={{ color: '#3A3D44', userSelect: 'none' }}>{String(i).padStart(4, '0')}</span>
          <span
            style={{
              color: line.includes('ERR') || line.includes('CRIT') ? '#FF453A' : '#6E7380',
            }}
          >
            {line}
            {i === lines.length - 1 && (
              <span style={{ opacity: cursor ? 1 : 0, color: '#00D4FF' }}>▌</span>
            )}
          </span>
        </div>
      ))}
    </div>
  );
}

/**
 * DataGridItem — minimal telemetry readout cell
 */
export function DataGridItem({
  label,
  value,
  sub,
}: {
  label: string;
  value: string | number;
  sub?: string;
}) {
  return (
    <div className="neo-inset rounded-xl p-3 transition-colors hover:border-white/[0.08]">
      <span className="label-xs block">{label}</span>
      <div className="data-value mt-1.5 text-base font-semibold text-[#E8EAF0]">{value}</div>
      {sub && <div className="label-xs mt-1 opacity-60">{sub}</div>}
    </div>
  );
}
