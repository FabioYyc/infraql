// variables
local name = 'kubernetes-the-hard-way';
local project = 'infraql-demo';

{
  // global config
  global: {
    project: project
  },
  // network
  network: {
    autoCreateSubnetworks: false,
    name: name + '-vpc',
    routingConfig: {routingMode: 'REGIONAL'}
  }
}