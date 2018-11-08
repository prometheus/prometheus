import {component} from 'flightjs';
import $ from 'jquery';

const NavbarUI = component(function navbar() {
  this.onNavigate = function(ev, {route}) {
    this.$node.find('[data-route]').each((i, el) => {
      const $el = $(el);
      if ($el.data('route') === route) {
        $el.addClass('active');
      } else {
        $el.removeClass('active');
      }
    });
  };

  this.after('initialize', function() {
    this.on(document, 'navigate', this.onNavigate);
  });
});

export default NavbarUI;
