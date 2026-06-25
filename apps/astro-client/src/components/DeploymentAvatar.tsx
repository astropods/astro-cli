import { AvatarImage } from "@/components/AvatarImage";
import { useDeploymentAvatarUrl } from "@/lib/avatar-bust";

export interface DeploymentAvatarProps {
  deployment: { id: string; name: string; avatar_url?: string };
  size?: number;
  className?: string;
}

/**
 * Renders a deployment's avatar. Owns the upload override + server-first URL
 * resolution (keyed on the deployment, not the blueprint) so callers pass the
 * deployment object and never rebuild the avatar key themselves.
 */
export function DeploymentAvatar({ deployment, size, className }: DeploymentAvatarProps) {
  const src = useDeploymentAvatarUrl(deployment);
  return <AvatarImage src={src} alt={deployment.name} size={size} className={className} />;
}
