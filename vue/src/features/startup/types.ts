export type StartupState = {
  phase: 'checking' | 'confirmation' | 'upgrading' | 'blocked' | 'ready';
  kind: string;
  current: number;
  target: number;
  databasePath: string;
  backupPath: string;
  message: string;
  detail: string;
};
