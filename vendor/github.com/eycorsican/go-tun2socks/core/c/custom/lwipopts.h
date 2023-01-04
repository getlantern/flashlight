/**
 * @file lwipopts.h
 * @author Ambroz Bizjak <ambrop7@gmail.com>
 * 
 * @section LICENSE
 * 
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 * 3. Neither the name of the author nor the
 *    names of its contributors may be used to endorse or promote products
 *    derived from this software without specific prior written permission.
 * 
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
 * ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
 * DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
 * LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
 * ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
 * SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

#ifndef LWIP_CUSTOM_LWIPOPTS_H
#define LWIP_CUSTOM_LWIPOPTS_H

// enable tun2socks logic
#define TUN2SOCKS 1

#if !defined NO_SYS
#define NO_SYS 1
#endif
#if !defined LWIP_TIMERS
#define LWIP_TIMERS 1
#endif

#if !defined LWIP_NOASSERT
#define LWIP_NOASSERT 1
#endif

#if !defined IP_DEFAULT_TTL
#define IP_DEFAULT_TTL 64
#endif
#if !defined LWIP_ARP
#define LWIP_ARP 0
#endif
#if !defined ARP_QUEUEING
#define ARP_QUEUEING 0
#endif
#if !defined IP_FORWARD
#define IP_FORWARD 0
#endif
#if !defined LWIP_ICMP
#define LWIP_ICMP 1
#endif
#if !defined LWIP_RAW
#define LWIP_RAW 1
#endif
#if !defined LWIP_DHCP
#define LWIP_DHCP 0
#endif
#if !defined LWIP_AUTOIP
#define LWIP_AUTOIP 0
#endif
#if !defined LWIP_SNMP
#define LWIP_SNMP 0
#endif
#if !defined LWIP_IGMP
#define LWIP_IGMP 0
#endif
#if !defined LWIP_DNS
#define LWIP_DNS 0
#endif
#if !defined LWIP_UDP
#define LWIP_UDP 1
#endif
#if !defined LWIP_UDPLITE
#define LWIP_UDPLITE 0
#endif
#if !defined LWIP_TCP
#define LWIP_TCP 1
#endif
#if !defined LWIP_CALLBACK_API
#define LWIP_CALLBACK_API 1
#endif
#if !defined LWIP_NETIF_API
#define LWIP_NETIF_API 0
#endif
#if !defined LWIP_NETIF_LOOPBACK
#define LWIP_NETIF_LOOPBACK 0
#endif
#if !defined LWIP_HAVE_LOOPIF
#define LWIP_HAVE_LOOPIF 1
#endif
#if !defined LWIP_HAVE_SLIPIF
#define LWIP_HAVE_SLIPIF 0
#endif
#if !defined LWIP_NETCONN
#define LWIP_NETCONN 0
#endif
#if !defined LWIP_SOCKET
#define LWIP_SOCKET 0
#endif
#if !defined PPP_SUPPORT
#define PPP_SUPPORT 0
#endif
#define LWIP_IPV6 1
#define LWIP_IPV6_MLD 0
#define LWIP_IPV6_AUTOCONFIG 1

// disable checksum checks
#if !defined CHECKSUM_CHECK_IP
#define CHECKSUM_CHECK_IP 0
#endif
#if !defined CHECKSUM_CHECK_UDP
#define CHECKSUM_CHECK_UDP 0
#endif
#if !defined CHECKSUM_CHECK_TCP
#define CHECKSUM_CHECK_TCP 0
#endif
#if !defined CHECKSUM_CHECK_ICMP
#define CHECKSUM_CHECK_ICMP 0
#endif
#define CHECKSUM_CHECK_ICMP6 0

#if !defined LWIP_CHECKSUM_ON_COPY
#define LWIP_CHECKSUM_ON_COPY 1
#endif

#if !defined MEMP_NUM_TCP_PCB_LISTEN
#define MEMP_NUM_TCP_PCB_LISTEN 1
#endif
#if !defined MEMP_NUM_TCP_PCB
#define MEMP_NUM_TCP_PCB 16
#endif
#if !defined MEMP_NUM_UDP_PCB
#define MEMP_NUM_UDP_PCB 1
#endif

/*
#if !defined TCP_LISTEN_BACKLOG
#define TCP_LISTEN_BACKLOG 1
#endif
#if !defined TCP_DEFAULT_LISTEN_BACKLOG
#define TCP_DEFAULT_LISTEN_BACKLOG 0xff
#endif
#if !defined LWIP_TCP_TIMESTAMPS
#define LWIP_TCP_TIMESTAMPS 1
#endif
*/

#if !defined TCP_MSS
#define TCP_MSS 1460
#endif
#if !defined TCP_WND
#define TCP_WND (2 * TCP_MSS)
#endif
#if !defined TCP_SND_BUF
#define TCP_SND_BUF (TCP_WND)
#endif
// #if !defined TCP_SND_QUEUELEN
// #define TCP_SND_QUEUELEN (2 * TCP_SND_BUF/TCP_MSS)
// #endif
// #if !defined TCP_SNDQUEUELOWAT
// #define TCP_SNDQUEUELOWAT (TCP_SND_QUEUELEN - 1)
// #endif

#if !defined MEM_LIBC_MALLOC
#define MEM_LIBC_MALLOC 0
#endif
#if !defined MEMP_MEM_MALLOC
#define MEMP_MEM_MALLOC 1
#endif
// #if !defined MEM_USE_POOLS
// #define MEM_USE_POOLS 0
// #endif
// #if !defined MEMP_USE_CUSTOM_POOLS
// #define MEMP_USE_CUSTOM_POOLS 1
// #endif

#if !defined MEM_SIZE
#define MEM_SIZE 1024768
#endif

#if !defined LWIP_STATS
#define LWIP_STATS 0
#endif
#if !defined LWIP_STATS_DISPLAY
#define LWIP_STATS_DISPLAY 0
#endif

#if !defined SYS_LIGHTWEIGHT_PROT
#define SYS_LIGHTWEIGHT_PROT 0
#endif
#define LWIP_DONT_PROVIDE_BYTEORDER_FUNCTIONS

// needed on 64-bit systems, enable it always so that the same configuration
// is used regardless of the platform
#define IPV6_FRAG_COPYHEADER 1

#if !defined LWIP_DEBUG
#define LWIP_DEBUG 0
#endif
#if !defined LWIP_DBG_TYPES_ON
#define LWIP_DBG_TYPES_ON LWIP_DBG_OFF
#endif
#if !defined INET_DEBUG
#define INET_DEBUG LWIP_DBG_ON
#endif
#if !defined IP_DEBUG
#define IP_DEBUG LWIP_DBG_ON
#endif
#if !defined RAW_DEBUG
#define RAW_DEBUG LWIP_DBG_ON
#endif
#if !defined SYS_DEBUG
#define SYS_DEBUG LWIP_DBG_ON
#endif
#if !defined NETIF_DEBUG
#define NETIF_DEBUG LWIP_DBG_ON
#endif
#if !defined TCP_DEBUG
#define TCP_DEBUG LWIP_DBG_ON
#endif
#if !defined UDP_DEBUG
#define UDP_DEBUG LWIP_DBG_ON
#endif
#if !defined TCP_INPUT_DEBUG
#define TCP_INPUT_DEBUG LWIP_DBG_ON
#endif
#if !defined TCP_OUTPUT_DEBUG
#define TCP_OUTPUT_DEBUG LWIP_DBG_ON
#endif
#if !defined TCPIP_DEBUG
#define TCPIP_DEBUG LWIP_DBG_ON
#endif
#define IP6_DEBUG LWIP_DBG_ON

#if !defined LWIP_STATS
#define LWIP_STATS 0
#endif
#if !defined LWIP_STATS_DISPLAY
#define LWIP_STATS_DISPLAY 0
#endif
#if !defined LWIP_PERF
#define LWIP_PERF 0
#endif

#endif
