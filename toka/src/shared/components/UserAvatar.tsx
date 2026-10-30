import React from "react";
import { Avatar, AvatarFallback, AvatarImage } from "../ui/avatar";
import { getInitials } from "../utils/initials";
import { getAvatarThumbUrl } from "../media";

type Props = {
  name?: string | null;
  avatarUrl?: string | null;
  className?: string;
  fallbackClassName?: string;
};

export function UserAvatar({ name, avatarUrl, className, fallbackClassName }: Props) {
  const initials = getInitials(name);
  const src = avatarUrl ? getAvatarThumbUrl(avatarUrl, 64) : undefined;

  return (
    <Avatar className={className}>
      {src ? <AvatarImage src={src} alt={name || "Avatar"} /> : null}
      <AvatarFallback className={fallbackClassName}>{initials}</AvatarFallback>
    </Avatar>
  );
}
