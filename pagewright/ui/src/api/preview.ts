// Never navigate before activation succeeds. The caller retains a clickable URL
// because browsers can block an async window.open even after a user gesture.
export async function openActivatedPreview(deploy: () => Promise<string>, open: (url: string) => void, active: () => boolean) {
  const url = await deploy();
  if (!active()) return undefined;
  try { open(url); } catch { /* Keep the confirmed URL for the fallback link. */ }
  return url;
}
