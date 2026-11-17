"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const {spawnSync} = require("node:child_process");
const root=path.resolve(__dirname,"..");
const targets=["windows-amd64","windows-arm64","linux-amd64","linux-arm64","darwin-amd64","darwin-arm64"];
const current=`${process.platform==="win32"?"windows":process.platform}-${process.arch==="x64"?"amd64":"arm64"}`;
const tools=["generate_image","generate_music","web_search","webpage_read","zagent_in_chrome"];
function binary(name){return path.join(root,name,"binaries",current,name+(process.platform==="win32"?".exe":""));}
function invoke(name,args){const result=spawnSync(binary(name),["--agent-json"],{input:JSON.stringify(args),encoding:"utf8",windowsHide:true});assert.equal(result.status,0,result.stderr);return JSON.parse(result.stdout);}

test("all platform manifests launch their neighboring binary without a runtime",()=>{
 for(const name of tools){
  const canonical=fs.readFileSync(path.join(root,"agent_tools",name,name+".json"),"utf8");
  for(const platform of targets){
   const folder=path.join(root,name,"binaries",platform);
   const manifest=JSON.parse(fs.readFileSync(path.join(folder,name+".json"),"utf8"));
   assert.deepEqual(manifest.command,[`{dir}/${name}`,"--agent-json"]);
   assert.equal(manifest.input,"stdin_json");assert.equal(manifest.output,"json");
   assert.equal(manifest.env,undefined);
   assert.equal(fs.readFileSync(path.join(folder,name+".json"),"utf8"),canonical);
   assert.ok(fs.existsSync(path.join(folder,name+(platform.startsWith("windows")?".exe":""))));
  }
 }
});

test("manifests cover every actual CLI flag",()=>{
 for(const name of tools){
  const manifest=JSON.parse(fs.readFileSync(path.join(root,"agent_tools",name,name+".json"),"utf8"));
  const help=spawnSync(binary(name),["--help"],{encoding:"utf8",windowsHide:true});assert.equal(help.status,0,help.stderr);
  const flags=[...help.stderr.matchAll(/^\s{2,}-([a-z_]+)(?:\s|$)/gm)].map(m=>m[1]);assert.ok(flags.length>10);
  for(const flag of flags)assert.ok(manifest.parameters.properties[flag],`${name}: missing ${flag}`);
  for(const [key,property] of Object.entries(manifest.parameters.properties))assert.ok(property.description?.length>5,`${name}.${key}`);
  assert.equal(manifest.parameters.additionalProperties,false);
 }
});

test("native binaries handle JSON directly, including inspection and errors",()=>{
 for(const name of tools){
  const version=invoke(name,{version:true});assert.equal(version.is_error,false,version.output);assert.match(version.output,/2\.0\.0/);
  const invalid=invoke(name,{unexpected:true});assert.equal(invalid.is_error,true);assert.match(invalid.output,/unknown argument/);
 }
 const help=invoke("web_search",{help:true});assert.equal(help.is_error,false);assert.match(help.output,/Usage: web_search/);
 const wrong=invoke("web_search",{query:"x",fallback:"false"});assert.equal(wrong.is_error,true);assert.match(wrong.output,/boolean/);
});
