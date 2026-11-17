export interface ApprovalOwner { readonly manager: string; readonly codexHome: string }
export function managedToolApproval(server: any): any;
export function isGestaltManagedMcp(name: string, server: any, owner: ApprovalOwner): boolean;
export function managedMcpApprovals(config: any, owner: ApprovalOwner): any;
export function managedMcpOverrides(config: any, baseline: any, owner: ApprovalOwner): any;
