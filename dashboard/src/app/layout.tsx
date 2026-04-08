import type { Metadata } from "next";
import { Inter, Orbitron, IBM_Plex_Mono, Rajdhani } from "next/font/google";
import "./globals.css";
import DashboardShell from "@/components/layout/DashboardShell";

const inter = Inter({ subsets: ["latin"], variable: "--font-inter" });
const orbitron = Orbitron({ subsets: ["latin"], variable: "--font-hud" });
const plex = IBM_Plex_Mono({ subsets: ["latin"], weight: ["400", "500", "600", "700"], variable: "--font-data" });
const rajdhani = Rajdhani({ subsets: ["latin"], weight: ["400", "500", "600", "700"], variable: "--font-lbl" });

export const metadata: Metadata = {
  title: "loadequilibrium | systems command",
  description: "NASA grade autonomic control dashboard",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${inter.variable} ${orbitron.variable} ${plex.variable} ${rajdhani.variable}`}>
       <body className="bg-[#010206] selection:bg-cyan-500/30">
        {/* HUD BACKGROUND LAYERS */}
        <div className="fixed inset-0 z-0 bg-grid-parallax opacity-30 pointer-events-none" />
        <div className="fixed inset-0 z-0 bg-scanline pointer-events-none" />
        <div className="fixed inset-0 z-0 bg-[radial-gradient(circle_at_center,transparent_30%,rgba(1,2,6,0.8)_100%)] pointer-events-none" />
        
        <DashboardShell>
          {children}
        </DashboardShell>
      </body>
    </html>
  );
}
