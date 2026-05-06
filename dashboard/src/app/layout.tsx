import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Control Plane Dashboard",
  description: "Enterprise control plane for distributed systems",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body className="bg-surface-0 text-text-primary antialiased">{children}</body>
    </html>
  );
}
