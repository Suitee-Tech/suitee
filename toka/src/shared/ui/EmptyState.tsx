import React from 'react';
import { LucideIcon } from 'lucide-react';

interface EmptyStateProps {
 icon: LucideIcon;
 title?: string;
 message: string;
 action?: React.ReactNode;
 className?: string;
}

export function EmptyState({ icon: Icon, title, message, action, className = '' }: EmptyStateProps) {
 return (
 <div className={`bg-card rounded-2xl border border-border p-12 flex flex-col items-center justify-center text-center ${className}`}>
 <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center mb-4 border border-border">
 <Icon className="w-8 h-8 text-muted-foreground/60" strokeWidth={1.5} />
 </div>
 {title && <h3 className="text-lg font-semibold text-foreground mb-2">{title}</h3>}
 <p className="text-sm text-muted-foreground max-w-sm">{message}</p>
 {action && <div className="mt-6">{action}</div>}
 </div>
 );
}
