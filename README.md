
# Gregor

[![Build Status](https://travis-ci.org/keybase/gregor.svg?branch=master)](https://travis-ci.org/keybase/gregor)

Gregor is a simple system for consuming, persisting, and broadcasting
notifications to a collection clients for a user.

## Architecture

```
                                            ┌──────────────────────────────────────────────┐    
                                            │                                              │    
                                            ▼                                              │    
              ┌──────────────────────────────────────────────────────────┐                 │    
              │                             │                    Server  │                 │    
              │                             │                            │                 │    
              │       ┌─────────────────────┘                            │                 │    
              │       │                       ┌───────────────────┐      │                 │    
              │       ▼                       │                   │      │                      
              │  ┌─────────┐  Consume         │    SQL Backend    │      │   Inject new message 
              │  │ Message │─────────────────▶│  (StateMachine)   │      │                      
              │  └─────────┘                  │                   │      │                 │    
              │       │                       └───────────────────┘      │                 │    
              │       │                                                  │                 │    
              │       │(copy)                                            │                 │    
              │       │                       ┌───────────────────┐      │                 │    
              │       ▼                       │                   │      │                 │    
              │  ┌─────────┐  Consume         │    Broadcaster    │      │                 │    
              │  │ Message │─────────────────▶│  (StateMachine)   │      │                 │    
              │  └─────────┘                  │                   │      │                 │    
              │                               └───────────────────┘      │                 │    
              │                                         │                │                 │    
              └─────────────────────────────────────────┼────────────────┘                 │    
                                                        │                                  │    
                                                        │                                  │    
                                                        │                                  │    
          ┌─────────────────────┬──────────────────────┬┴─────────────────────┐            │    
          │                     │                      │                      │            │    
          │                     │                      │                      │            │    
          │Consume              │Consume               │Consume        Consume│            │    
          │                     │                      │                      │            │    
          ▼                     ▼                      ▼                      ▼            │    
┌───────────────────┐ ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐  │    
│                   │ │                   │  │                   │  │                   │  │    
│   Client Store    │ │   Client Store    │  │   Client Store    │  │   Client Store    │  │    
│  (StateMachine)   │ │  (StateMachine)   │  │  (StateMachine)   │  │  (StateMachine)   │──┘    
│                   │ │                   │  │                   │  │                   │       
└─────────┬─────────┤ └─────────┬─────────┤  └─────────┬─────────┤  └─────────┬─────────┤       
          │device 1 │           │device 2 │            │device 3 │            │device 4 │       
          └─────────┘           └─────────┘            └─────────┘            └─────────┘       
```
