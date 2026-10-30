"use client";

import * as React from "react";
import * as AvatarPrimitive from "@radix-ui/react-avatar";

import { getMediaUrl } from "../media";
import { cn } from "./utils";

function Avatar({
 className,
 ...props
}: React.ComponentProps<typeof AvatarPrimitive.Root>) {
 return (
 <AvatarPrimitive.Root
 data-slot="avatar"
 className={cn(
 "relative flex size-10 shrink-0 overflow-hidden rounded-full",
 className,
 )}
 {...props}
 />
 );
}

function AvatarImage({
 className,
 src,
 ...props
}: React.ComponentProps<typeof AvatarPrimitive.Image>) {
 const resolvedSrc = typeof src === "string" ? getMediaUrl(src) : src;

 return (
 <AvatarPrimitive.Image
 data-slot="avatar-image"
  loading="lazy"
  decoding="async"
 className={cn("aspect-square size-full object-cover", className)}
 src={resolvedSrc || undefined}
 {...props}
 />
 );
}

function AvatarFallback({
 className,
 ...props
}: React.ComponentProps<typeof AvatarPrimitive.Fallback>) {
 return (
 <AvatarPrimitive.Fallback
 data-slot="avatar-fallback"
 className={cn(
 "bg-muted flex size-full items-center justify-center rounded-full",
 className,
 )}
 {...props}
 />
 );
}

export { Avatar, AvatarImage, AvatarFallback };
