const header = document.querySelector('[data-header]');
const nav = document.querySelector('[data-nav]');
const toggle = document.querySelector('[data-nav-toggle]');

const updateHeader = () => header?.classList.toggle('scrolled', window.scrollY > 12);
updateHeader();
window.addEventListener('scroll', updateHeader, { passive: true });

toggle?.addEventListener('click', () => {
  const open = nav?.classList.toggle('open') ?? false;
  toggle.setAttribute('aria-expanded', String(open));
});

nav?.querySelectorAll('a').forEach((link) => link.addEventListener('click', () => {
  nav.classList.remove('open');
  toggle?.setAttribute('aria-expanded', 'false');
}));

document.querySelectorAll('[data-year]').forEach((node) => {
  node.textContent = String(new Date().getFullYear());
});

const items = document.querySelectorAll('.reveal');
if ('IntersectionObserver' in window) {
  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        entry.target.classList.add('visible');
        observer.unobserve(entry.target);
      }
    });
  }, { threshold: 0.08 });
  items.forEach((item) => observer.observe(item));
} else {
  items.forEach((item) => item.classList.add('visible'));
}
