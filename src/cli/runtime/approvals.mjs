// Existing managed transport/approval policy, shared by setup and runtime.
import { join } from 'node:path';
function managedToolApproval(server) {
  return {...server, ...(!server.url ? {env_vars:[...new Set([...(server.env_vars ?? []),'GESTALT_SESSION_TMP_CONTEXT','TMPDIR','TMP','TEMP'])]} : {}), default_tools_approval_mode:'approve', ...(server.tools ? {tools:Object.fromEntries(
    Object.entries(server.tools).map(([name, tool]) => [name, tool && typeof tool === 'object' &&
      Object.hasOwn(tool,'approval_mode') ? {...tool,approval_mode:'approve'} : tool]))} : {})};
}
function isGestaltManagedMcp(name, server, owner) {
  if (!server || typeof server !== 'object' || server.url) return false;
  const components = {'gestalt-serena':'serena','gestalt-playwright':'playwright'};
  if (Object.hasOwn(components,name)) return (!server.command && !server.args) ||
    ([owner.manager,'gestalt'].includes(server.command) && server.args?.[0] === components[name] && server.args?.[1] === 'mcp');
  return name === 'context-mode' && ['node','node.exe'].includes(String(server.command).replaceAll('\\','/').split('/').at(-1)) &&
    server.args?.includes(join(owner.codexHome, 'bin/context-mode-mcp.mjs'));
}
function managedMcpApprovals(config, owner) {
  const result = {...config};
  if (config.mcp_servers) result.mcp_servers = Object.fromEntries(Object.entries(config.mcp_servers)
    .map(([name,server]) => [name,isGestaltManagedMcp(name,server,owner) ? managedToolApproval(server) : server]));
  if (config.plugins) result.plugins = Object.fromEntries(Object.entries(config.plugins).map(([name,plugin]) =>
    [name,name === 'context-mode@dyne-gestalt-agents' && plugin?.mcp_servers ? {...plugin,mcp_servers:Object.fromEntries(
      Object.entries(plugin.mcp_servers).map(([serverName,server]) => [serverName,serverName === 'context-mode' ? managedToolApproval(server) : server]))} : plugin]));
  return result;
}

function managedMcpOverrides(config, baseline, owner) {
  const object = value => value && typeof value === 'object' && !Array.isArray(value);
  const omitAbsent = value => Array.isArray(value) ? value.map(omitAbsent) : object(value) ?
    Object.fromEntries(Object.entries(value).filter(([,v]) => v !== null).map(([k,v]) => [k,omitAbsent(v)])) : value;
  const merge = (base, options) => {
    const result = {...base};
    if (Object.hasOwn(options,'url') && !Object.hasOwn(options,'command'))
      for (const key of ['command','args','cwd','env','env_vars']) delete result[key];
    if (Object.hasOwn(options,'command') && !Object.hasOwn(options,'url')) delete result.url;
    for (const [key,value] of Object.entries(options)) Object.defineProperty(result,key,{value:
      object(value) ? merge(object(result[key]) ? result[key] : {},value) : value,enumerable:true,configurable:true,writable:true});
    return result;
  };
  // Preserve effective native transports and enablement. Absent inherited nulls
  // cannot be round-tripped through Codex's JSON-to-TOML config overrides.
  const result = {...config};
  for (const section of ['mcp_servers','plugins']) {
    let effective = merge(omitAbsent(baseline?.[section] ?? {}),config?.[section] ?? {});
    for (const key of Object.keys(result).filter(key => key.startsWith(section+'.')).sort()) {
      let value = result[key];
      for (const part of key.slice(section.length+1).split('.').reverse()) value = {[part]:value};
      effective = merge(effective,value);
    }
    const approved = managedMcpApprovals({[section]:effective},owner)[section];
    // Do not serialize unrelated inherited settings/credentials into overrides.
    const changed = Object.fromEntries(Object.entries(approved).filter(([name,server]) =>
      JSON.stringify(server) !== JSON.stringify(effective[name])));
    for (const name of Object.keys(changed)) for (const key of Object.keys(result))
      if (key === section+'.'+name || key.startsWith(section+'.'+name+'.')) delete result[key];
    if (Object.keys(changed).length || config?.[section]) result[section] = merge(config?.[section] ?? {},changed);
  }
  return result;
}

export { managedToolApproval, isGestaltManagedMcp, managedMcpApprovals, managedMcpOverrides };
