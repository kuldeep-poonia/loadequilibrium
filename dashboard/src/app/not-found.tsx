import Link from 'next/link';
import { TacticalBox } from '@/components/ui/HUD';

export default function NotFound() {
  return (
    <div className="h-full flex items-center justify-center">
      <TacticalBox title="SEC::404_PAGE_NOT_FOUND" badge="CRITICAL_ERROR" className="max-w-md">
        <div className="text-center p-6">
          <h2 className="text-2xl font-hud font-black text-red-500 mb-6 tracking-widest uppercase">Target Vector Missing</h2>
          <p className="text-[11px] text-slate-500 font-hud tracking-[0.1em] leading-relaxed mb-10">
            The requested module or coordination point does not exist in the loadequilibrium system map. Vector alignment failed.
          </p>
          <Link 
            href="/telemetry" 
            className="inline-block px-8 py-3 bg-cyan-500/10 border border-cyan-400 text-cyan-400 font-hud text-[11px] uppercase tracking-widest hover:bg-cyan-500/20 transition-all"
          >
            RETURN_TO_COMMAND
          </Link>
        </div>
      </TacticalBox>
    </div>
  );
}
