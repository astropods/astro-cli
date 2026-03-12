export function deploymentPath(account: string, deploymentId: string) {
  return `/${account}/agents/${deploymentId}`;
}

export function deploymentConfigurePath(account: string, deploymentId: string) {
  return `${deploymentPath(account, deploymentId)}/configure`;
}
