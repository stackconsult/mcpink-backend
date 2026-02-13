# Machines

| Name            | IPv4            | Private IP (vSwitch) | SSH Command                |
| --------------- | --------------- | -------------------- | -------------------------- |
| hetzner-factory | 46.225.65.56    | 10.0.1.2             | `ssh root@46.225.65.56`    |
| run-1           | 157.90.130.187  | 10.0.1.3             | `ssh root@157.90.130.187`  |
| ops-1           | 116.202.163.209 | 10.0.1.4             | `ssh root@116.202.163.209` |
| build-1         | 46.225.92.127   | 10.0.0.3             | `ssh root@46.225.92.127`   |
| k3s-1           | 46.225.100.234  | 10.0.0.4             | `ssh root@46.225.100.234`  |

## vSwitch Private Network

All servers communicate over a private vSwitch network for internal traffic (registry pulls, monitoring, etc.).

| Setting              | Value       |
| -------------------- | ----------- |
| **VLAN ID**          | 4000        |
| **Cloud Network ID** | #11898981   |
| **Subnet**           | 10.0.1.0/24 |
| **Gateway**          | 10.0.1.1    |

### Netplan Configuration (Dedicated Servers)

For dedicated servers (run-1, ops-1, builder-1), configure vSwitch via netplan:

```yaml
# /etc/netplan/50-vswitch.yaml
network:
  version: 2
  vlans:
    vlan4000:
      id: 4000
      link: enp0s31f6 # Check actual interface with: ip link show
      addresses:
        - 10.0.1.X/24 # Use assigned IP
      routes:
        - to: 10.0.1.0/24
          via: 10.0.1.1
```

Factory (Cloud VPS) is attached to Cloud Network #11898981 via Hetzner Cloud Console.

## Hardware Specifications

### run-1

**Dedicated Server (Server Auction)**

- **CPU:** AMD EPYC 7502P
- **Storage:** 2x SSD U.2 NVMe 1.92 TB Datacenter
- **RAM:** 8x 32768 MB DDR4 ECC reg. (256 GB total)
- **Network:** NIC 1 Gbit Intel I350
- **Location:** Germany, FSN1
- **IP:** 1x Primary IPv4
- **Role:** Run user containers (MCP servers)

### hetzner-factory

**VPS**

- **CPU:** 4 vCPU
- **RAM:** 8 GB
- **Storage:** 160 GB local disk
- **Volume:** volume-factory-1 (100 GB)
- **Location:** Germany, Nuremberg
- **Role:** Coolify master

### build-1

**Hetzner Cloud VPS**

- **CPU:** 4 vCPU (AMD)
- **RAM:** 16 GB
- **Storage:** 305 GB local disk
- **Volume:** 200 GB (mounted at `/mnt/HC_Volume_104561676`, used for Docker)
- **Location:** Germany, Nuremberg
- **Role:** Build server for Coolify

### ops-1

**Dedicated Server (Server Auction #2893003)**

- **CPU:** Intel Xeon E-2176G
- **Storage:** 2x 960GB Datacenter U.2 NVMe + 2x 2TB HDD
- **RAM:** ECC (amount TBD)
- **Location:** Germany, FSN1-DC15
- **Role:** Docker Registry, Gitea/Forgejo, Grafana, Prometheus

**Storage Layout:**
| Mount | RAID | Drives | Purpose |
|-------|------|--------|---------|
| `/` + `/data` | RAID1 | 2×960GB NVMe | OS + Docker + Registry + Gitea |
| `/backups` | RAID1 | 2×2TB HDD | Local backup staging |

### k3s-1

**Hetzner Cloud VPS (CX33)**

- **CPU:** x86
- **Storage:** 80 GB local disk
- **Location:** Germany, Nuremberg (eu-central)
- **Role:** K3s cluster node
