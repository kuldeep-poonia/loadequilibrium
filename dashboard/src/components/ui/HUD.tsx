'use client';

import { useEffect, useState } from 'react';
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { motion, type HTMLMotionProps } from 'framer-motion';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

function panelRef(title?: string, badge?: string) {
  const seed = `${title || 'panel'}:${badge || 'status'}`;
  const hash = seed
    .split('')
    .reduce((acc, char) => ((acc * 33) ^ char.charCodeAt(0)) >>> 0, 5381);

  return hash.toString(36).toUpperCase().padStart(6, '0').slice(0, 6);
}

/**
 * TACTICAL_CORNER :: Laser-cut aesthetic
 */
export function TacticalCorner({ position, color = 'cyan' }: { position: 'tl' | 'tr' | 'bl' | 'br', color?: 'cyan' | 'red' | 'amber' }) {
  const colorMap = {
    cyan: 'border-cyan-400/35 shadow-[0_0_0_1px_rgba(34,211,238,0.05)]',
    red: 'border-red-400/35 shadow-[0_0_0_1px_rgba(248,113,113,0.05)]',
    amber: 'border-amber-400/35 shadow-[0_0_0_1px_rgba(245,158,11,0.05)]',
  };
  
  const posClasses = {
    tl: 'top-[-1px] left-[-1px] border-r-0 border-b-0 rounded-tl-[4px]',
    tr: 'top-[-1px] right-[-1px] border-l-0 border-b-0 rounded-tr-[4px]',
    bl: 'bottom-[-1px] left-[-1px] border-r-0 border-t-0 rounded-bl-[4px]',
    br: 'bottom-[-1px] right-[-1px] border-l-0 border-t-0 rounded-br-[4px]',
  };

  return (
    <div className={cn(
      "absolute w-4 h-4 border z-20 pointer-events-none transition-all duration-500",
      colorMap[color],
      posClasses[position]
    )} />
  );
}

/**
 * TACTICAL_BOX :: The Primary Command Frame
 */
export function TacticalBox({ 
  children, 
  className, 
  title, 
  badge, 
  status = 'nominal',
  scan = true,
  ...props
}: { 
  children: React.ReactNode, 
  className?: string, 
  title?: string, 
  badge?: string,
  status?: 'nominal' | 'warning' | 'critical' | 'alert',
  scan?: boolean 
} & HTMLMotionProps<"div">) {
  const statusColors: Record<string, "cyan" | "amber" | "red"> = {
    nominal: 'cyan',
    warning: 'amber',
    critical: 'red',
    alert: 'red',
  };
  const refCode = panelRef(title, badge);

  return (
    <motion.div 
      className={cn(
        "relative flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[24px] border bg-[#121822] shadow-[18px_18px_36px_rgba(3,6,11,0.55),_-10px_-10px_26px_rgba(29,36,48,0.16),_inset_0_1px_0_rgba(255,255,255,0.03)] transition-all duration-500",
        status === 'critical'
          ? 'border-red-500/20'
          : status === 'warning'
            ? 'border-amber-500/20'
            : 'border-white/6',
        className
      )}
      {...props}
    >
      {/* HUD DECORATIONS */}
      <TacticalCorner position="tl" color={statusColors[status]} />
      <TacticalCorner position="tr" color={statusColors[status]} />
      <TacticalCorner position="bl" color={statusColors[status]} />
      <TacticalCorner position="br" color={statusColors[status]} />
      
      {/* SCANNING LINE (OPTIONAL) */}
      {scan && (
        <div className="absolute inset-0 z-10 pointer-events-none overflow-hidden opacity-[0.04]">
          <div className="absolute left-0 top-0 h-px w-full bg-white/60 animate-[bg-scan_12s_linear_infinite]" />
        </div>
      )}

      {/* HEADER STRIP */}
      {title && (
        <div className="relative z-20 flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-b border-white/6 bg-[#151c27] px-3.5 py-2 shadow-[inset_0_-1px_0_rgba(0,0,0,0.35)] xl:px-4">
          <div className="flex min-w-0 flex-1 items-center gap-3">
             <div className={cn(
               "h-3 w-1 rounded-full",
               status === 'nominal'
                 ? 'bg-cyan-300'
                 : status === 'warning'
                   ? 'bg-amber-300'
                   : 'bg-red-300'
             )} />
             <span title={title} className="min-w-0 truncate whitespace-nowrap font-hud text-[8px] font-black uppercase tracking-[0.22em] text-slate-100">{title}</span>
          </div>
          {badge && (
            <div className="ml-auto flex max-w-full shrink-0 items-center gap-2">
               <span className="hidden text-[7px] font-hud tracking-[0.18em] text-slate-600 2xl:inline">REF::{refCode}</span>
               <span className={cn(
                 "rounded-full border px-2 py-1 text-[8px] font-bold font-hud uppercase tracking-[0.2em] shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]",
                 status === 'nominal'
                   ? 'border-cyan-500/20 bg-[#10161f] text-cyan-200'
                   : status === 'warning'
                     ? 'border-amber-500/20 bg-[#10161f] text-amber-200'
                     : 'border-red-500/20 bg-[#10161f] text-red-200'
               )}>
                 {badge}
               </span>
            </div>
          )}
        </div>
      )}
      
      {/* CONTENT AREA */}
      <div className="relative z-20 flex min-h-0 min-w-0 flex-1 overflow-hidden p-3 xl:p-4">
        {children}
      </div>
    </motion.div>
  );
}

/**
 * STATUS_TUBE :: Glowing saturation gauge
 */
export function StatusTube({ 
  label, 
  value, 
  percent, 
  vertical = false,
  status = 'nominal'
}: { 
  label: string, 
  value: string | number, 
  percent: number, 
  vertical?: boolean,
  status?: 'nominal' | 'warning' | 'critical'
}) {
  const safePercent = Math.min(100, Math.max(0, percent * 100));
  
  return (
    <div className={cn("flex gap-4", vertical ? "flex-col items-center" : "flex-col")}>
      <div className="flex justify-between items-end w-full px-1">
        <div className="flex flex-col">
          <span className="text-[7px] font-hud text-slate-500 uppercase tracking-widest">{label}</span>
          <span className={cn("text-[14px] font-bold font-data", status === 'critical' ? 'text-hud-red' : 'text-hud-cyan')}>
            {value}
          </span>
        </div>
        <span className="text-[9px] font-hud text-slate-400 opacity-50">{Math.round(safePercent)}%</span>
      </div>

      <div className={cn(
        "relative rounded-sm overflow-hidden bg-slate-900 shadow-inner",
        vertical ? "w-4 h-32" : "w-full h-2.5"
      )}>
        {/* TUBE BACKGROUND DASHES */}
        <div className="absolute inset-0 flex gap-1 px-1 py-0.5 opacity-20">
          {[...Array(10)].map((_, i) => (
            <div key={i} className="flex-1 bg-white/10" />
          ))}
        </div>

        {/* TUBE GLOW */}
        <div 
          className={cn(
            "absolute transition-all duration-1000 ease-out shadow-[0_0_15px_rgba(0,242,255,0.4)]",
            status === 'critical' ? 'bg-hud-red shadow-hud-red/40' : 'bg-hud-cyan shadow-hud-cyan/40',
            vertical ? "bottom-0 w-full" : "top-0 h-full"
          )} 
          style={{ [vertical ? 'height' : 'width']: `${safePercent}%` }}
        />
        
        {/* TUBE GLASS REFLECTION */}
        <div className="absolute inset-0 bg-gradient-to-t from-transparent via-white/10 to-transparent pointer-events-none" />
      </div>
    </div>
  );
}

/**
 * DIGITAL_LOG :: Tactical Console Feed
 */
export function DigitalLog({ lines }: { lines: string[] }) {
  const [jitter, setJitter] = useState(false);
  
  useEffect(() => {
    const itv = setInterval(() => setJitter(prev => !prev), 100);
    return () => clearInterval(itv);
  }, []);

  return (
    <div className="font-data text-[10px] leading-relaxed text-slate-400 space-y-1.5 h-full overflow-y-auto pr-2 scrollbar-hud">
      {lines.map((line, i) => (
        <div key={i} className="flex gap-3 group">
          <span className="text-slate-600 font-hud opacity-50 select-none">
            {String(i).padStart(4, '0')}
          </span>
          <span className={cn(
            "transition-opacity duration-75",
            jitter && i === lines.length - 1 ? 'opacity-50' : 'opacity-100',
            line.includes('ERR') || line.includes('CRIT') ? 'text-hud-red font-bold' : 'group-hover:text-hud-cyan'
          )}>
            {"> "} {line}
          </span>
        </div>
      ))}
    </div>
  );
}

/**
 * DATA_GRID_ITEM :: Technical Readout
 */
export function DataGridItem({ label, value, sub }: { label: string, value: string | number, sub?: string }) {
  return (
    <div className="border border-white/[0.03] bg-white/[0.01] p-3 hover:bg-hud-cyan/5 transition-all group">
      <div className="flex justify-between items-center mb-1">
        <span className="text-[8px] font-hud text-slate-500 uppercase tracking-widest group-hover:text-hud-cyan transition-colors">
          {label}
        </span>
        <div className="w-1 h-1 bg-slate-700 rounded-full" />
      </div>
      <div className="text-lg font-bold text-slate-200 tabular-nums">
        {value}
      </div>
      {sub && (
        <div className="text-[7px] font-hud text-slate-600 uppercase tracking-tighter mt-1">
          {sub}
        </div>
      )}
    </div>
  );
}
