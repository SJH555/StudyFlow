export type StartupState = {
  phase: 'checking' | 'confirmation' | 'upgrading' | 'blocked' | 'ready';
  kind: string;
  systemZone?: string;
  current: number;
  target: number;
  databasePath: string;
  backupPath: string;
  message: string;
  detail: string;
};
