# Billing overview

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

Astro measures what an account uses, bills it, and stops it if it does not pay.
This page is the short version. It takes about five minutes.

Read this first if you are new to billing, or if you need to answer one question
and do not want the whole design. For the full picture, read
[`billing-architecture.md`](billing-architecture.md). For the code path with
file references, read [`billing-data-flow.md`](billing-data-flow.md).

## What Astro does and does not do

Astro measures usage and reports it. Astro never computes a price and never
charges a card.

Three external systems each own one job:

| System | Owns |
|---|---|
| Metronome | rating usage, closing billing periods, producing invoices |
| Stripe | holding the card and charging it |
| Novu | sending the emails |

## The path a dollar takes

1. Every five minutes, astro-server measures the compute each deployment
   reserved and sends it to Metronome as usage events.
2. The AI gateway sends its own token usage to Metronome, separately.
3. Metronome rates the usage against the account's contract, closes the period,
   and produces an invoice.
4. Metronome sends the invoice to Stripe. Stripe charges the saved card.
5. If the charge fails, Stripe sends a webhook. astro-server records it and may
   suspend the account.

## Plans

Every account sits on one of two packages. Both share the same rate card,
so usage meters identically. They differ only in what is attached.

| Plan | Who gets it | What it means |
|---|---|---|
| Signup credit | a new customer, first account | Draws down the signup credit, then bills the card. |
| Pay as you go | a returning customer's second account | Bills the card from the start. |

The signup credit belongs to a person, not an account. Someone who creates a
second organization does not get a second credit, and deleting an account does
not return it.

The billing settings page shows which plan an account is on.

## How an account gets suspended

astro-server does not check a balance when a request arrives. That would be slow
and would depend on Metronome being up. Instead it keeps one row per account and
reads that.

Provider webhooks write facts to the row. Four facts matter: credit is spent, a
spend limit was crossed, an invoice was written off, and a payment is overdue. A
pure function turns those facts into one status: `active`, `past_due`, or
`suspended`.

A suspended account cannot deploy, and its running agents are stopped.

Two rules are worth knowing:

- **Spent credit only suspends an account with no card on file.** With a card,
  spending the credit is just the move to pay as you go.
- **`BILLING_GATE_ENFORCE` chooses enforce or observe.** In observe mode the
  server logs the block it would have made and allows the request.

## How an account recovers

Recovery is always an event, never a side effect of the next read. Paying the
invoice, resolving the alert, or an operator granting credit clears the latch,
and the account's agents start again.

Adding a card is not by itself recovery. It stops the credit gate, because that
gate only applies without a card, but it does not clear an unpaid invoice.

## What a customer controls

An account can set two numbers on itself:

- A **warning** tells the owner and changes nothing.
- A **limit** stops every agent in the account.

Both live in Metronome, not in our database, so the page reads through to the
number that will actually fire.

## Where to look when something is wrong

Start in astro-queen's billing view. It shows the account's status and the reason
it was gated, and it links straight to that account's Metronome customer page.

**The Metronome dashboard** answers everything about measuring and rating:

- the customer, and the ingest aliases usage attributes through
- every usage event, searchable by customer or transaction ID
- the open period's draft invoice, and every finalized one
- which contract and package the customer is on, which is their plan
- each alert and whether it is currently in alarm

**The Stripe dashboard** answers everything about collecting the money:

- the customer and the saved card
- each invoice Metronome sent, and whether it was paid, failed, or written off
- the payment attempts behind a failure, including a 3DS prompt the customer
  never completed
- webhook deliveries, so you can tell a missing event from an ignored one

If a fact spans both, match them by the invoice: Metronome's invoice carries an
`external_invoice` field holding the Stripe invoice it was delivered to. An empty
one means the invoice never reached Stripe, which is a delivery problem rather
than a payment one.

For emails, use Novu's activity feed for the account.

If you cannot sign in to either dashboard, ask a billing owner to add you. Both
are per environment, so access to the sandbox is not access to production.
