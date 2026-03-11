This is a temporary fork of the Accordion component from Mantine UI v8.3.6 with modifications specific to Prometheus.
The component has been modified to unmount children of collapsed panels to reduce page rendering times and
resource usage.

According to Mantine author Vitaly, a similar feature has now been added to Mantine itself, but will only be
available in version 9.0.0 and later, quote from https://discord.com/channels/854810300876062770/1006447791498870784/threads/1428787320546525336:

> I've managed to implement it, but only in 9.0 since it requires some breaking changes. Will be available next year

So this Accordion fork can be removed once Prometheus upgrades to Mantine v9 or later.
