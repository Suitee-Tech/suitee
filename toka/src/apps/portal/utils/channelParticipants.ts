import type { Channel, PortalParticipant } from '../hooks/usePortalChannels';

export function getPortalParticipants(channel: Channel): PortalParticipant[] {
  const profiles = new Map((channel.participant_profiles || []).map((profile) => [profile.id, profile]));
  return (Array.isArray(channel.participants) ? channel.participants : []).map((participant) => {
    const base = typeof participant === 'string' ? { id: participant } : participant;
    return { ...base, ...profiles.get(base.id) };
  });
}
