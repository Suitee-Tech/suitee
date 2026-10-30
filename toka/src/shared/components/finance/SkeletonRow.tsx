import { Skeleton } from "../../ui/skeleton";

interface SkeletonRowProps {
 cols: number;
}

export function SkeletonRow({ cols }: SkeletonRowProps) {
 return (
 <tr className="border-b border-border/40">
 {Array.from({ length: cols }).map((_, i) => (
 <td key={i} className="py-4 px-4">
 <Skeleton className="h-4 w-full max-w-[160px]" />
 </td>
 ))}
 </tr>
 );
}
