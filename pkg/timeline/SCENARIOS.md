# SCENARIOS

## High Level

## Real Use Cases ...

Range of Anlysis ...
t0 - tn
Per Group of Customers ..
Per Exact Customer ...


## Deep Analysis #1
===============

- Cohorts (all combo for features):
a) GeoA, CDN1, vNew
b) GeoB, CDN2, vNew
c) ... combo ..
d) GeoB, CDN2, vOld

Output: In time range; all events related to all users in each Cohort .. compare agaonst same timing ..

WorkflowID: <Cohort-Combo>
- Starting Data State
- Flow the events in batch as Signals in ..
- When not selected then end of that timing tn
- Sample at t0, tn-5, tn
- Stateful calculate the bufferings for each customers (child workflow?) rollup
- Buffer >5s is flagged as bad at observation time 

Flow:
- Orchestrator ingest batches of 30s data ..related only to CUJ
- Validate it it only: CUJ - Customer-Using-Video-Player
- It knows who needs which data and will pass it on etc ..
- ChildWorkflow: <Cohort-Combo>-<UUID>


## Deep Analysis #2

SCENARIO: Instead, teams must be able to analyze fine-grained “micro” cohorts—such as Chrome users searching for shoes in 
California in experiment group C while logged in. 

Cohort Features:

CUJ: Product Searching
- Browser
- Location
- Experiment Group
- Product Category
- Login State

Meta:

- DeviceID
- CF_Country
- BotScore

Stateful Analysis: 

- How long to start to reading details
- How many Bookmark
- How many Add To Cart
- HOw many add to Agent Shopper
- Session Length
- Drop-Off Ratio

UserGroup as FingerPrint:

- Feature flags
- A/B test variants
- Campaign IDs
- Subscription tiers
- Payment providers
-  Device, OS, app version, geo

A micro-cohort is a fingerprint:
A unique combination of technical, behavioral, and business-specific attributes that reveals who’s struggling—and why.

Think:

    Device type (e.g., Android 12)
    App version (e.g., 5.1)
    Region (e.g., Midwest)
    Loyalty tier (e.g., Premium Plus)
    A/B test group (e.g., Checkout Flow v2 Group B)
    Feature flag exposure
    Logged-in status
    Subscription plan
    Referral source

MUST be able to answer:

    Who exactly is affected?
    Where are they located?
    What do they have in common?
    How big is the business impact?

Goal: 

- Product Teams + Owners own discovery via self-service
- Real-Time Performance Analytics—built to unify behavior, experience, and backend performance in one live, decision-ready view.

Charactristic:

- Define new metrics without writing a line of code
- Create flows visually, without tagging
- Ask questions and get real-time answers
- Test and learn in minutes—not weeks

How?

- Specify time Analysis - Timeline Producers - State snapshot at time T0
- Define flows (yaml Temporal Workflow) - Timeline Operators
- Build metrics (Timeline Operators to Extract view at event Tn) - Digester
- Get answers instantly; streaming from the Digester

Now imagine opening your dashboard and immediately seeing:

    Where the drop began
    Who is affected, such as iOS 18.3 users on app version 5.1 in California
    What is causing it, such as a third-party payment API that slowed overnight
    How much it is costing you, like $26,000 per day in lost checkouts

And imagine finding it in 60 seconds, 

