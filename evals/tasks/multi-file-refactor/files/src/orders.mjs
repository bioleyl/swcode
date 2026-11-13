const formatDate = (iso) => {
  const [year, month, day] = iso.split('-');
  return `${day}.${month}.${year}`;
};

export const orderLine = (order) => `#${order.id} on ${formatDate(order.placed)}`;
