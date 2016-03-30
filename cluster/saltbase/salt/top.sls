base:
  '*':
    - base
    - debian-auto-upgrades
    - salt-helpers
{% if grains.get('cloud') == 'aws' %}
    - ntp
{% endif %}
{% if pillar.get('e2e_storage_test_environment', '').lower() == 'true' %}
    - e2e
{% endif %}

  'roles:kubernetes-pool':
    - match: grain
    - docker
{% if pillar.get('network_provider', '').lower() == 'flannel' %}
    - flannel
{% elif pillar.get('network_provider', '').lower() == 'kubenet' %}
    - cni
{% endif %}
    - helpers
    - kube-client-tools
    - kube-node-unpacker
    - kubelet
{% if pillar.get('network_provider', '').lower() == 'opencontrail' %}
    - opencontrail-networking-minion
{% else %}
    - kube-proxy
{% endif %}
{% if pillar.get('enable_cluster_registry', '').lower() == 'true' %}
    - kube-registry-proxy
{% endif %}
    - logrotate
    - supervisor

  'roles:kubernetes-master':
    - match: grain
    - generate-cert
    - etcd
{% if pillar.get('network_provider', '').lower() == 'flannel' %}
    - flannel-server
    - flannel
{% elif pillar.get('network_provider', '').lower() == 'kubenet' %}
    - cni
{% endif %}
    - kube-apiserver
    - kube-controller-manager
    - kube-scheduler
    - supervisor
    - kube-client-tools
    - kube-master-addons
    - kube-admission-controls
{% if grains['cloud'] is defined and grains['cloud'] != 'vagrant' %}
    - logrotate
{% endif %}
    - kube-addons
{% if grains['cloud'] is defined and grains['cloud'] in [ 'vagrant', 'gce', 'aws', 'vsphere' ] %}
    - docker
    - kubelet
{% endif %}
{% if pillar.get('network_provider', '').lower() == 'opencontrail' %}
    - opencontrail-networking-master
{% endif %}
