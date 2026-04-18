import type { Metadata } from 'next';
import './globals.css';
import DashboardShell from '@/components/layout/DashboardShell';

export const metadata: Metadata = {
  title: 'LoadEquilibrium · Control Room',
  description: 'Convergence, conservation, signal integrity and topology stability',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" style={{ background: '#000000' }}>
      <body>
        <DashboardShell>{children}</DashboardShell>
      </body>
    </html>
  );
}
